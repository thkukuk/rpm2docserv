package main

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/thkukuk/rpm2docserv/pkg/bundled"
	"github.com/thkukuk/rpm2docserv/pkg/commontmpl"
	"github.com/thkukuk/rpm2docserv/pkg/convert"
	"github.com/thkukuk/rpm2docserv/pkg/manpage"
	"github.com/thkukuk/rpm2docserv/pkg/write"
	"golang.org/x/text/language"
)

const iso8601Format = "2006-01-02T15:04:05Z"

var sortOrder = make(map[string]int)

// stapelberg came up with the following abbreviations:
var shortSections = map[string]string{
	"1": "progs",
	"2": "syscalls",
	"3": "libfuncs",
	"4": "files",
	"5": "formats",
	"6": "games",
	"7": "misc",
	"8": "sysadmin",
	"9": "kernel",
}

// taken from man(1)
var longSections = map[string]string{
	"1": "Executable programs or shell commands",
	"2": "System calls (functions provided by the kernel)",
	"3": "Library calls (functions within program libraries)",
	"4": "Special files (usually found in /dev)",
	"5": "File formats and conventions eg /etc/passwd",
	"6": "Games",
	"7": "Miscellaneous (including macro packages and conventions), e.g. man(7), groff(7)",
	"8": "System administration commands (usually only for root)",
	"9": "Kernel routines [Non standard]",
}

var manpageTmpl = mustParseManpageTmpl()

func mustParseManpageTmpl() *template.Template {
	return template.Must(template.Must(commonTmpls.Clone()).New("manpage").
		Funcs(map[string]interface{}{
			"ShortSection": func(section string) string {
				return shortSections[section]
			},
			"LongSection": func(section string) string {
				return longSections[section]
			},
			"FragmentLink": func(fragment string) string {
				u := url.URL{Fragment: strings.Replace(fragment, " ", "_", -1)}
				return u.String()
			},
		}).
		Parse(bundled.Asset("manpage.tmpl")))
}

var manpageerrorTmpl = mustParseManpageerrorTmpl()

func mustParseManpageerrorTmpl() *template.Template {
	return template.Must(template.Must(commonTmpls.Clone()).New("manpage-error").
		Funcs(map[string]interface{}{
			"ShortSection": func(section string) string {
				return shortSections[section]
			},
			"LongSection": func(section string) string {
				return longSections[section]
			},
			"FragmentLink": func(fragment string) string {
				u := url.URL{Fragment: strings.Replace(fragment, " ", "_", -1)}
				return u.String()
			},
		}).
		Parse(bundled.Asset("manpageerror.tmpl")))
}

var manpagefooterextraTmpl = mustParseManpagefooterextraTmpl()

func mustParseManpagefooterextraTmpl() *template.Template {
	return template.Must(template.Must(commonTmpls.Clone()).New("manpage-footerextra").
		Funcs(map[string]interface{}{
			"Iso8601": func(t time.Time) string {
				return t.UTC().Format(iso8601Format)
			},
		}).
		Parse(bundled.Asset("manpagefooterextra.tmpl")))
}

func convertFile(src string, resolve func(ref string) string) (doc string, toc []string, err error) {
	f, err := os.Open(src)
	if err != nil {
		return "", nil, err
	}
	defer f.Close()

	r := io.Reader(f)
	gzipr, err := gzip.NewReader(f)
	if err != nil {
		if err == io.EOF {
			// TODO: better representation of an empty manpage
			return "This space intentionally left blank.", nil, nil
		} else if err != gzip.ErrHeader {
			return "", nil, err
		}
	} else {
		r = gzipr
		defer gzipr.Close()
	}
	out, toc, err := convert.ToHTML(r, resolve)
	if err != nil {
		return "", nil, fmt.Errorf("convert(%q): %v", src, err)
	}
	return out, toc, nil
}

type byPkgAndLanguage struct {
	opts       []*manpage.Meta
	currentpkg string
}

func (p byPkgAndLanguage) Len() int      { return len(p.opts) }
func (p byPkgAndLanguage) Swap(i, j int) { p.opts[i], p.opts[j] = p.opts[j], p.opts[i] }
func (p byPkgAndLanguage) Less(i, j int) bool {
	// prefer manpages from the same package
	if p.opts[i].Package.Binarypkg != p.opts[j].Package.Binarypkg {
		if p.opts[i].Package.Binarypkg == p.currentpkg {
			return true
		}
	}
	return p.opts[i].Language < p.opts[j].Language
}

// bestLanguageMatch returns the best manpage out of options (coming
// from current) based on text/language’s matching.
func bestLanguageMatch(current *manpage.Meta, options []*manpage.Meta) *manpage.Meta {
	sort.Stable(byPkgAndLanguage{options, current.Package.Binarypkg})

	if options[0].Language != "en" {
		for i := 1; i < len(options); i++ {
			if options[i].Language == "en" {
				options = append([]*manpage.Meta{options[i]}, options...)
				break
			}
		}
	}

	tags := make([]language.Tag, len(options))
	for idx, m := range options {
		tags[idx] = m.LanguageTag
	}

	matcher := language.NewMatcher(tags)
	_, idx, _ := matcher.Match(current.LanguageTag)
	return options[idx]
}

type byLanguage []*manpage.Meta

func (p byLanguage) Len() int           { return len(p) }
func (p byLanguage) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p byLanguage) Less(i, j int) bool { return p[i].Language < p[j].Language }

type renderJob struct {
	dest     string
	src      string
	meta     *manpage.Meta
	versions []*manpage.Meta
	xref     map[string][]*manpage.Meta
	modTime  time.Time
}

var notYetRenderedSentinel = errors.New("Not yet rendered")

type manpagePrepData struct {
	Title          string
	ProjectName    string
	ProjectUrl     string
	LogoUrl        string
	IsOffline      bool
	Rpm2docservVersion string
	Breadcrumbs    breadcrumbs
	FooterExtra    template.HTML
	AltVersions    []*manpage.Meta
	Versions       []*manpage.Meta
	Sections       []*manpage.Meta
	Bins           []*manpage.Meta
	Langs          []*manpage.Meta
	HrefLangs      []*manpage.Meta
	Meta           *manpage.Meta
	TOC            []string
	Ambiguous      map[*manpage.Meta]bool
	Content        template.HTML
	Error          error
	Products       []string
}

type byProduct []*manpage.Meta

func (p byProduct) Len() int      { return len(p) }
func (p byProduct) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p byProduct) Less(i, j int) bool {
	orderi, oki := sortOrder[p[i].Package.Product]
	orderj, okj := sortOrder[p[j].Package.Product]
	if !oki || !okj {
		// if we have a known suite, prefer that over the unknown one
		if oki && !okj {
			return true
		}
		if okj && !oki {
			return false
		}
		return p[i].Package.Product < p[j].Package.Product
	}
	return orderi < orderj
}

type byMainSection []*manpage.Meta

func (p byMainSection) Len() int           { return len(p) }
func (p byMainSection) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p byMainSection) Less(i, j int) bool { return p[i].MainSection() < p[j].MainSection() }

type byBinarypkg []*manpage.Meta

func (p byBinarypkg) Len() int           { return len(p) }
func (p byBinarypkg) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p byBinarypkg) Less(i, j int) bool { return p[i].Package.Binarypkg < p[j].Package.Binarypkg }

func rendermanpageprep(job renderJob, gv *globalView) (*template.Template, manpagePrepData, error) {
	meta := job.meta // for convenience
	// TODO(issue): document fundamental limitation: “other languages” is imprecise: e.g. crontab(1) — are the languages for package:systemd-cron or for package:cron?
	// TODO(later): to boost confidence in detecting cross-references, can we add to testdata the entire list of man page names from debian to have a good test?
	// TODO(later): add plain-text version

	var (
		content   string
		toc       []string
		renderErr = notYetRenderedSentinel
	)

	content, toc, renderErr = convertFile(job.src, func(ref string) string {
		idx := strings.LastIndex(ref, "(")
		if idx == -1 {
			return ""
		}
		section := ref[idx+1 : len(ref)-1]
		name := ref[:idx]
		related, ok := job.xref[name]
		if !ok {
			return ""
		}
		filtered := make([]*manpage.Meta, 0, len(related))
		for _, r := range related {
			if r.MainSection() != section {
				continue
			}
			if r.Package.Product != meta.Package.Product {
				continue
			}
			filtered = append(filtered, r)
		}
		if len(filtered) == 0 {
			return ""
		}
		return commontmpl.BaseURLPath() + "/" + bestLanguageMatch(meta, filtered).ServingPath() + ".html"
	})
	if renderErr != nil {
		log.Printf("ERROR: Rendering %q failed: %q", job.dest, renderErr)
	}

	if *verbose {
		log.Printf("rendering %q", job.dest)
	}

	altVersions := make([]*manpage.Meta, 0, len(job.versions))
	for _, v := range job.versions {
		if !v.Package.SameBinary(meta.Package) {
			continue
		}
		if v.Section != meta.Section {
			continue
		}
		// TODO(later): allow switching to a different suite even if
		// switching requires a language-change. we should indicate
		// this in the UI.
		if v.Language != meta.Language {
			continue
		}
		altVersions = append(altVersions, v)
	}

	sort.Stable(byProduct(altVersions))

	bySection := make(map[string][]*manpage.Meta)
	for _, v := range job.versions {
		if v.Package.Product != meta.Package.Product {
			continue
		}
		bySection[v.Section] = append(bySection[v.Section], v)
	}
	sections := make([]*manpage.Meta, 0, len(bySection))
	for _, all := range bySection {
		sections = append(sections, bestLanguageMatch(meta, all))
	}
	sort.Stable(byMainSection(sections))

	conflicting := make(map[string]bool)
	bins := make([]*manpage.Meta, 0, len(job.versions))
	for _, v := range job.versions {
		if v.Section != meta.Section {
			continue
		}

		if v.Package.Product != meta.Package.Product {
			continue
		}

		// We require a strict match for the language when determining
		// conflicting packages, because otherwise the packages might
		// be augmenting, not conflicting: crontab(1) is present in
		// cron, but its translations are shipped e.g. in
		// manpages-fr-extra.
		if v.Language != meta.Language {
			continue
		}

		if v.Package.Binarypkg != meta.Package.Binarypkg {
			conflicting[v.Package.Binarypkg] = true
		}
		bins = append(bins, v)
	}
	sort.Stable(byBinarypkg(bins))

	ambiguous := make(map[*manpage.Meta]bool)
	byLang := make(map[string][]*manpage.Meta)
	for _, v := range job.versions {
		if v.Section != meta.Section {
			continue
		}
		if v.Package.Product != meta.Package.Product {
			continue
		}
		if conflicting[v.Package.Binarypkg] {
			continue
		}

		byLang[v.Language] = append(byLang[v.Language], v)
	}
	langs := make([]*manpage.Meta, 0, len(byLang))
	hrefLangs := make([]*manpage.Meta, 0, len(byLang))
	for _, all := range byLang {
		for _, e := range all {
			langs = append(langs, e)
			if len(all) > 1 {
				ambiguous[e] = true
			}
			// hreflang consists only of language and region,
			// scripts are not supported.
			if !strings.Contains(e.Language, "@") {
				hrefLangs = append(hrefLangs, e)
			}
		}
	}

	// Sort alphabetically by the locale names (e.g. zh_TW).
	sort.Sort(byLanguage(langs))
	sort.Sort(byLanguage(hrefLangs))

	t := manpageTmpl
	title := fmt.Sprintf("%s(%s) — %s", meta.Name, meta.Section, meta.Package.Binarypkg)
	shorttitle := fmt.Sprintf("%s(%s)", meta.Name, meta.Section)
	if renderErr != nil {
		t = manpageerrorTmpl
		title = "Error: " + title
	}

	var footerExtra bytes.Buffer
	if err := manpagefooterextraTmpl.Execute(&footerExtra, struct {
		SourceFile  string
		LastUpdated time.Time
		Converted   time.Time
		Meta        *manpage.Meta
	}{
		SourceFile:  filepath.Base(job.src),
		LastUpdated: job.modTime,
		Converted:   time.Now(),
		Meta:        meta,
	}); err != nil {
		return nil, manpagePrepData{}, err
	}

	return t, manpagePrepData{
		Title:          title,
		ProjectName:    projectName,
		ProjectUrl:     projectUrl,
		LogoUrl:        logoUrl,
		IsOffline:      isOffline,
		Rpm2docservVersion: rpm2docservVersion,
		Breadcrumbs: breadcrumbs{
			{fmt.Sprintf("/%s/index.html", meta.Package.Product), meta.Package.Product},
			{fmt.Sprintf("/%s/%s/index.html", meta.Package.Product, meta.Package.Binarypkg), meta.Package.Binarypkg},
			{"", shorttitle},
		},
		FooterExtra: template.HTML(footerExtra.String()),
		AltVersions: altVersions,
		Versions:    job.versions,
		Sections:    sections,
		Bins:        bins,
		Langs:       langs,
		HrefLangs:   hrefLangs,
		Meta:        meta,
		TOC:         toc,
		Ambiguous:   ambiguous,
		Content:     template.HTML(content),
		Error:       renderErr,
		Products:    gv.productList,
	}, nil
}

type countingWriter int64

func (c *countingWriter) Write(p []byte) (n int, err error) {
	*c += countingWriter(len(p))
	return len(p), nil
}

func rendermanpage(gzipw *gzip.Writer, job renderJob, gv *globalView) (uint64, error) {
	t, data, err := rendermanpageprep(job, gv)
	if err != nil {
		return 0, err
	}

	var written countingWriter
	if err := write.AtomicallyWithGz(job.dest, gzipw, func(w io.Writer) error {
		return t.Execute(io.MultiWriter(w, &written), data)
	}); err != nil {
		return 0, err
	}

	return uint64(written), nil
}
