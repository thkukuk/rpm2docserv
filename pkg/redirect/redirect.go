package redirect

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"os"
	"slices"
	"sort"
	"strings"

	pb "github.com/thkukuk/rpm2docserv/pkg/proto"
	"github.com/thkukuk/rpm2docserv/pkg/tag"
	"google.golang.org/protobuf/proto"
	"golang.org/x/text/language"
	//"golang.org/x/text/language/display"
)

type IndexEntry struct {
	Name      string // TODO: string pool
	Product   string // TODO: enum to save space
	Binarypkg string // TODO: sort by popcon, TODO: use a string pool
	Section   string // TODO: use a string pool
	Language  string // TODO: type: would it make sense to use language.Tag?
}

func (e IndexEntry) ServingPath(suffix string) string {
	return "/" + e.Product + "/" + e.Binarypkg + "/" + e.Name + "." + e.Section + "." + e.Language + suffix
}

type Index struct {
	Entries        map[string][]IndexEntry
	ProductNames   []string
	Langs          []string
	Sections       []string
	ProductMapping map[string]string
}

func bestLanguageMatch(t []language.Tag, options []IndexEntry) IndexEntry {
	// if no preferred language is set, use english
	if t == nil {
		t = []language.Tag{language.English}
	}

	tags := make([]language.Tag, len(options))
	for idx, m := range options {
		tag, err := tag.FromLocale(m.Language)
		if err != nil {
			panic(fmt.Sprintf("Cannot get language.Tag from locale %q: %v", m.Language, err))
		}
		tags[idx] = tag
	}

	matcher := language.NewMatcher(tags)
	_, idx, confidence := matcher.Match(t...)
	//log.Printf("best match: %s (%s) index=%d confidence=%v", display.English.Tags().Name(tag),
        //	   display.Self.Name(tag), idx, confidence)
	if confidence == language.Exact {
	        return options[idx]
	}
	return options[0]
}

func (i Index) split(path string) (product string, binarypkg string, name string, section string, lang string) {
	dir := strings.TrimPrefix(filepath.Dir(path), "/")
	base := strings.TrimSpace(filepath.Base(path))
	base = strings.Replace(base, " ", ".", -1)
	parts := strings.Split(dir, "/")
	if len(parts) > 0 {
		if len(parts) == 1 {
			if _, ok := i.ProductMapping[parts[0]]; ok {
				product = parts[0]
			} else {
				if sliceContainsSorted(i.Sections,base) {
					// man.freebsd.org
					section = base
					base = parts[0]
				} else {
					binarypkg = parts[0]
				}
			}
		} else if len(parts) == 2 {
			product = parts[0]
			binarypkg = parts[1]
		}
	}

	// the first part can contain dots, so we need to “split from the right”
	parts = strings.Split(base, ".")
	if len(parts) == 1 {
		return product, binarypkg, base, section, lang
	}

	// The last part can either be a language or a section
	consumed := 0
	if l := parts[len(parts)-1]; sliceContainsSorted(i.Langs, l) {
		lang = l
		consumed++
	} else if l := parts[len(parts)-1]; sliceContainsSorted(i.Sections, l) {
		section = l
		consumed++
	}
	// The second to last part (if enough parts are present) can
	// be a section (because the language was already specified).
	if len(parts) > 1+consumed {
		if s := parts[len(parts)-1-consumed]; sliceContainsSorted(i.Sections, s) {
			section = s
			consumed++
		}
	}

	return product,
		binarypkg,
		strings.Join(parts[:len(parts)-consumed], "."),
		section,
		lang
}

// Default taken from man(1):
var mansect = searchOrder(strings.Split("0 1 n l 8 3 2 5 4 9 6 7 1x 3x 4x 5x 6x 8x 1bind 3bind 5bind 7bind 8bind 1cn 8cn 1m 1mh 5mh 8mh 1netpbm 3netpbm 5netpbm 0p 1p 3p 3posix 1pgsql 3pgsql 5pgsql 3C++ 8C++ 3blt 3curses 3ncurses 3form 3menu 3db 3gdbm 3f 3gk 3paper 3mm 5mm 3perl 3pm 3pq 3qt 3pub 3readline 1ssl 3ssl 5ssl 7ssl 3t 3tk 3tcl 3tclx 3tix 7l 7nr 8c Cg g s m", " "))

func searchOrder(sections []string) map[string]int {
	order := make(map[string]int)
	for idx, section := range sections {
		order[section] = idx
	}
	return order
}

type bySection []IndexEntry

func (p bySection) Len() int      { return len(p) }
func (p bySection) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p bySection) Less(i, j int) bool {
	oI, okI := mansect[p[i].Section]
	oJ, okJ := mansect[p[j].Section]
	if okI && okJ { // both sections are in mansect
		return oI < oJ
	}
	if !okI && okJ {
		return false // sort all mansect sections before custom sections
	}
	if okI && !okJ {
		return true // sort all mansect sections before custom sections
	}
	return p[i].Section < p[j].Section // neither are in mansect
}

func sliceContainsSorted(slice []string, lookup string) bool {
	_, found := slices.BinarySearch(slice, lookup)
	return found
}

func (i Index) Narrow(acceptLang string, query, referrer IndexEntry, entries []IndexEntry) []IndexEntry {
	q := query // for convenience

	fullyQualified := func() bool {
		if q.Product == "" || q.Binarypkg == "" || q.Section == "" || q.Language == "" {
			return false
		}

		// Verify validity
		for _, e := range entries {
			if q.Product == e.Product &&
				q.Binarypkg == e.Binarypkg &&
				q.Section == e.Section &&
				q.Language == e.Language {
				return true
			}
		}
		return false
	}

	filtered := make([]IndexEntry, len(entries))
	copy(filtered, entries)

	filter := func(keep func(e IndexEntry) bool) {
		tmp := filtered[:0]
		for _, e := range filtered {
			if !keep(e) {
				continue
			}
			tmp = append(tmp, e)
		}
		filtered = tmp
	}

	// Narrow down as much as possible upfront. The keep callback is
	// the logical and of all the keep callbacks below:
	filter(func(e IndexEntry) bool {
		return (q.Product == "" || e.Product == q.Product) &&
			(q.Section == "" || e.Section[:1] == q.Section[:1]) &&
			(q.Language == "" || e.Language == q.Language) &&
			(q.Binarypkg == "" || e.Binarypkg == q.Binarypkg)
	})
	if len(filtered) == 0 {
		return nil
	}

	// suite

	if q.Product == "" {
		// Prefer redirecting to the suite from the referrer
		for _, e := range filtered {
			if e.Product == referrer.Product {
				q.Product = referrer.Product
				break
			}
		}
	}

	filter(func(e IndexEntry) bool { return q.Product == "" || e.Product == q.Product })
	if len(filtered) == 0 {
		return nil
	}
	if fullyQualified() {
		return filtered
	}

	// section

	// Sort by section following the order as used by man
	sort.Stable(bySection(filtered))

	if q.Section == "" {
		// Prefer section from the referrer
		for _, e := range filtered {
			if e.Section == referrer.Section {
				q.Section = referrer.Section
				break
			}
		}
		// If still empty, use first one
		if q.Section == "" {
			q.Section = filtered[0].Section
		}
	}

	filter(func(e IndexEntry) bool { return q.Section == "" || e.Section[:1] == q.Section[:1] })
	if len(filtered) == 0 {
		return nil
	}
	if fullyQualified() {
		return filtered
	}

	// language

	if q.Language == "" {
		tags, _, _ := language.ParseAcceptLanguage(acceptLang)
		// ignore err: tags == nil results in the default language
		best := bestLanguageMatch(tags, filtered)
		q.Language = best.Language
	}

	filter(func(e IndexEntry) bool { return q.Language == "" || e.Language == q.Language })
	if len(filtered) == 0 {
		return nil
	}
	if fullyQualified() {
		return filtered
	}

	// binarypkg

	if q.Binarypkg == "" {
		q.Binarypkg = filtered[0].Binarypkg
	}

	filter(func(e IndexEntry) bool { return q.Binarypkg == "" || e.Binarypkg == q.Binarypkg })
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

type NotFoundError struct {
	Manpage  string
	Choices  []IndexEntry
	Products []string
}

func (e *NotFoundError) Error() string {
	return "No such man page"
}

func (i Index) Redirect(r *http.Request) (string, error) {
	path := r.URL.Path

	if strings.HasSuffix(path, "/") ||
		strings.HasSuffix(path, "/index.html") ||
		strings.HasPrefix(path, "/contents-") {
		return "", &NotFoundError{}
	}

	suffix := ".html"
	// If a raw manpage was requested, redirect to raw, not HTML
	if strings.HasSuffix(path, ".gz") && !strings.HasSuffix(path, ".html.gz") {
		suffix = ".gz"
	}
	for strings.HasSuffix(path, ".html") || strings.HasSuffix(path, ".gz") {
		path = strings.TrimSuffix(path, ".gz")
		path = strings.TrimSuffix(path, ".html")
	}

	// Parens are converted into dots, so that “i3(1)” becomes
	// “i3.1.”. Trailing dots are stripped and two dots next to each
	// other are converted into one.
	path = strings.Replace(path, "(", ".", -1)
	path = strings.Replace(path, ")", ".", -1)
	path = strings.Replace(path, "..", ".", -1)
	path = strings.TrimSuffix(path, ".")

	suite, binarypkg, name, section, lang := i.split(path)

	if rewrite, ok := i.ProductMapping[suite]; ok {
		suite = rewrite
	}

	lname := strings.ToLower(name)
	entries, ok := i.Entries[lname]
	if !ok {
		// Fall back to joining (originally) whitespace-separated
		// parts by dashes and underscores, like man(1).
		entries, ok = i.Entries[strings.Replace(lname, ".", "-", -1)]
		if !ok {
			entries, ok = i.Entries[strings.Replace(lname, ".", "_", -1)]
			if !ok {
				log.Printf("Not found: Url %q, path %q", r.URL.Path, path)
				return "", &NotFoundError{Manpage: name}
			}
		}
	}

	log.Printf("Query %q, path %q -> suite = %q, binarypkg = %q, name = %q, section = %q, lang = %q", r.URL.Path, path, suite, binarypkg, name, section, lang)

	acceptLang := r.Header.Get("Accept-Language")
	referrer := IndexEntry{
		Product:   r.FormValue("suite"),
		Binarypkg: r.FormValue("binarypkg"),
		Section:   r.FormValue("section"),
		Language:  r.FormValue("language"),
	}
	filtered := i.Narrow(acceptLang, IndexEntry{
		Product:   suite,
		Binarypkg: binarypkg,
		Section:   section,
		Language:  lang,
	}, referrer, entries)

	if len(filtered) == 0 {
		// Present the user with another choice for this manpage.
		var choices []IndexEntry
		if name != "index" && name != "favicon" {
		        //choices = i.Narrow(acceptLang, IndexEntry{}, referrer, entries)
			choices = entries
		}
		log.Printf("Not found: Url %q, suggesting %q", r.URL.Path, choices)

		return "", &NotFoundError{
			Manpage:  name,
			Choices:  choices,
		        Products: i.ProductNames}
	}
	log.Printf("Found: Query %q -> Url %q", r.URL.Path, filtered[0].ServingPath(suffix))

	return filtered[0].ServingPath(suffix), nil
}

func IndexFromProto(paths []string) (Index, error) {
	index := Index{
		ProductMapping:   make(map[string]string),
	}
	var idx pb.Index

	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			return index, err
		}

		err = proto.UnmarshalOptions{Merge: true}.Unmarshal(b, &idx)
		if err != nil {
			return index, err
		}
	}

	index.Entries = make(map[string][]IndexEntry, len(idx.Entry))
	for _, e := range idx.Entry {
		name := strings.ToLower(e.Name)
		index.Entries[name] = append(index.Entries[name], IndexEntry{
			Name:      e.Name,
			Product:   e.Suite,
			Binarypkg: e.Binarypkg,
			Section:   e.Section,
			Language:  e.Language,
		})
	}
	index.Langs = idx.Language
	index.Sections = idx.Section
	index.ProductMapping = idx.Suite

	// old index files are not sorted
	sort.Strings(index.Langs)
	sort.Strings(index.Sections)

	if len(idx.Products) > 0 {
		index.ProductNames = idx.Products
	} else {
		// No product names in the index file, generate ourself
		// Sort order does not need to match rpm2docserv!
		index.ProductNames = make([]string, 0, len(idx.Suite))
		for i := range idx.Suite {
			index.ProductNames = append(index.ProductNames, idx.Suite[i])
		}
		sort.Strings(index.ProductNames)
		index.ProductNames = slices.Compact(index.ProductNames)
	}
	return index, nil
}
