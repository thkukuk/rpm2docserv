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

func renderPkgIndex(dest string, manpageByName map[string]*manpage.Meta, gv *globalView) error {
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

	return write.Atomically(dest, false, func(w io.Writer) error {
		return pkgindexTmpl.Execute(w, struct {
			Title          string
			ProjectName    string
			ProjectUrl     string
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
			Products       []string
		}{
			Title:          fmt.Sprintf("Manpages of %s", first.Package.Binarypkg),
			ProjectName:    projectName,
			ProjectUrl:     projectUrl,
			LogoUrl:        logoUrl,
			Rpm2docservVersion: rpm2docservVersion,
			IsOffline:      isOffline,
			Breadcrumbs: breadcrumbs{
				{fmt.Sprintf("/%s/index.html", first.Package.Product), first.Package.Product},
				{"", first.Package.Binarypkg},
			},
			First:         first,
			Meta:          first,
			ManpageByName: manpageByName,
			Mans:          mans,
			Products:      productList,
		})
	})
}

func renderSrcPkgIndex(dest string, src string,
	               manpageByName map[string]*manpage.Meta, gv *globalView) error {
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

	return write.Atomically(dest, false, func(w io.Writer) error {
		return srcpkgindexTmpl.Execute(w, struct {
			Title          string
			ProjectName    string
			ProjectUrl     string
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
			Products       []string
		}{
			Title:          fmt.Sprintf("Manpages of src:%s", src),
			ProjectName:    projectName,
			ProjectUrl:     projectUrl,
			LogoUrl:        logoUrl,
			IsOffline:      isOffline,
			Rpm2docservVersion: rpm2docservVersion,
			Breadcrumbs: breadcrumbs{
				{fmt.Sprintf("/%s/index.html", first.Package.Product), first.Package.Product},
				{"", "src:" + src},
			},
			First:         first,
			Meta:          first,
			ManpageByName: manpageByName,
			Mans:          mans,
			Src:           src,
			Products:      productList,
		})
	})
}
