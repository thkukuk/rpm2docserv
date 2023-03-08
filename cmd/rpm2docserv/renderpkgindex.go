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

func renderPkgindex(dest string, manpageByName map[string]*manpage.Meta, gv globalView) error {
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

	suites := make([]string, 0, len(gv.suites))
	for suite := range gv.suites {
		suites = append(suites, suite)
	}
	sort.Stable(bySuiteStr(suites))

	return write.Atomically(dest, false, func(w io.Writer) error {
		return pkgindexTmpl.Execute(w, struct {
			Title          string
			ProductName    string
			ProductUrl     string
			LogoUrl        string
			Rpm2docservVersion string
			IsOffline      bool
			Breadcrumbs    breadcrumbs
			FooterExtra    string
			First          *manpage.Meta
			Meta           *manpage.Meta
			ManpageByName  map[string]*manpage.Meta
			Mans           []string
			HrefLangs      []*manpage.Meta
			Suites         []string
		}{
			Title:          fmt.Sprintf("Manpages of %s", first.Package.Binarypkg),
			ProductName:    productName,
			ProductUrl:     productUrl,
			LogoUrl:        logoUrl,
			Rpm2docservVersion: rpm2docservVersion,
			IsOffline:      isOffline,
			Breadcrumbs: breadcrumbs{
				{fmt.Sprintf("/%s/index.html", first.Package.Suite), first.Package.Suite},
				{"", first.Package.Binarypkg},
			},
			First:         first,
			Meta:          first,
			ManpageByName: manpageByName,
			Mans:          mans,
			Suites:        suites,
		})
	})
}

func renderSrcPkgindex(dest string, src string,
	               manpageByName map[string]*manpage.Meta, gv globalView) error {
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

	suites := make([]string, 0, len(gv.suites))
	for suite := range gv.suites {
		suites = append(suites, suite)
	}
	sort.Stable(bySuiteStr(suites))

	return write.Atomically(dest, false, func(w io.Writer) error {
		return srcpkgindexTmpl.Execute(w, struct {
			Title          string
			ProductName    string
			ProductUrl     string
			LogoUrl        string
			IsOffline      bool
			Rpm2docservVersion string
			Breadcrumbs    breadcrumbs
			FooterExtra    string
			First          *manpage.Meta
			Meta           *manpage.Meta
			ManpageByName  map[string]*manpage.Meta
			Mans           []string
			HrefLangs      []*manpage.Meta
			Src            string
			Suites         []string
		}{
			Title:          fmt.Sprintf("Manpages of src:%s", src),
			ProductName:    productName,
			ProductUrl:     productUrl,
			LogoUrl:        logoUrl,
			IsOffline:      isOffline,
			Rpm2docservVersion: rpm2docservVersion,
			Breadcrumbs: breadcrumbs{
				{fmt.Sprintf("/%s/index.html", first.Package.Suite), first.Package.Suite},
				{"", "src:" + src},
			},
			First:         first,
			Meta:          first,
			ManpageByName: manpageByName,
			Mans:          mans,
			Src:           src,
			Suites:        suites,
		})
	})
}
