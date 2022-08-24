package main

import (
	"fmt"
	"html/template"
	"io"
	"sort"

	"github.com/thkukuk/rpm2docserv/pkg/bundled"
	"github.com/thkukuk/rpm2docserv/pkg/manpage"
	"github.com/thkukuk/rpm2docserv/pkg/write"
)

var pkgindexTmpl = mustParsePkgindexTmpl()
var srcpkgindexTmpl = mustParseSrcPkgindexTmpl()

func mustParsePkgindexTmpl() *template.Template {
	return template.Must(template.Must(commonTmpls.Clone()).New("pkgindex").Parse(bundled.Asset("pkgindex.tmpl")))
}

func mustParseSrcPkgindexTmpl() *template.Template {
	return template.Must(template.Must(commonTmpls.Clone()).New("srcpkgindex").Parse(bundled.Asset("srcpkgindex.tmpl")))
}

func renderPkgindex(dest string, manpageByName map[string]*manpage.Meta) error {
	var first *manpage.Meta
	for _, m := range manpageByName {
		first = m
		break
	}

	mans := make([]string, 0, len(manpageByName))
	for n := range manpageByName {
		mans = append(mans, n)
	}
	sort.Strings(mans)

	return write.Atomically(dest, true, func(w io.Writer) error {
		return pkgindexTmpl.Execute(w, struct {
			Title          string
			Rpm2docservVersion string
			Breadcrumbs    breadcrumbs
			FooterExtra    string
			First          *manpage.Meta
			Meta           *manpage.Meta
			ManpageByName  map[string]*manpage.Meta
			Mans           []string
			HrefLangs      []*manpage.Meta
		}{
			Title:          fmt.Sprintf("Manpages of %s", first.Package.Binarypkg),
			Rpm2docservVersion: rpm2docservVersion,
			Breadcrumbs: breadcrumbs{
				{fmt.Sprintf("/contents-%s.html", first.Package.Suite), first.Package.Suite},
				{fmt.Sprintf("/%s/%s/index.html", first.Package.Suite, first.Package.Binarypkg), first.Package.Binarypkg},
				{"", "Contents"},
			},
			First:         first,
			Meta:          first,
			ManpageByName: manpageByName,
			Mans:          mans,
		})
	})
}

func renderSrcPkgindex(dest string, src string, manpageByName map[string]*manpage.Meta) error {
	var first *manpage.Meta
	for _, m := range manpageByName {
		first = m
		break
	}

	mans := make([]string, 0, len(manpageByName))
	for n := range manpageByName {
		mans = append(mans, n)
	}
	sort.Strings(mans)

	return write.Atomically(dest, true, func(w io.Writer) error {
		return srcpkgindexTmpl.Execute(w, struct {
			Title          string
			Rpm2docservVersion string
			Breadcrumbs    breadcrumbs
			FooterExtra    string
			First          *manpage.Meta
			Meta           *manpage.Meta
			ManpageByName  map[string]*manpage.Meta
			Mans           []string
			HrefLangs      []*manpage.Meta
			Src            string
		}{
			Title:          fmt.Sprintf("Manpages of src:%s", src),
			Rpm2docservVersion: rpm2docservVersion,
			Breadcrumbs: breadcrumbs{
				{fmt.Sprintf("/contents-%s.html", first.Package.Suite), first.Package.Suite},
				{fmt.Sprintf("/%s/src:%s/index.html", first.Package.Suite, src), "src:" + src},
				{"", "Contents"},
			},
			First:         first,
			Meta:          first,
			ManpageByName: manpageByName,
			Mans:          mans,
			Src:           src,
		})
	})
}
