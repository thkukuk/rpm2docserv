package main

import (
	"fmt"
	"html/template"
	"io"

	"github.com/thkukuk/rpm2docserv/pkg/bundled"
	"github.com/thkukuk/rpm2docserv/pkg/manpage"
	"github.com/thkukuk/rpm2docserv/pkg/write"
)

var contentsTmpl = mustParseContentsTmpl()

func mustParseContentsTmpl() *template.Template {
	return template.Must(template.Must(commonTmpls.Clone()).New("contents").Parse(bundled.Asset("contents.tmpl")))
}

func renderProductContents(dest, productName string, pkgdirs []string, srcpkgdirs []string, gv globalView) error {
	if err := write.Atomically(dest, false, func(w io.Writer) error {
		return contentsTmpl.Execute(w, struct {
			Title          string
			ProjectName    string
			ProjectUrl     string
			LogoUrl        string
			Rpm2docservVersion string
			IsOffline      bool
			Breadcrumbs    breadcrumbs
			FooterExtra    string
			ProductName    string
			PkgDirs        []string
			SrcPkgDirs     []string
			Products       []string
			Meta           *manpage.Meta
			HrefLangs      []*manpage.Meta
		}{
			Title:          fmt.Sprintf("Manpages of %s", productName),
			ProjectName:    projectName,
			ProjectUrl:     projectUrl,
			LogoUrl:        logoUrl,
			IsOffline:      isOffline,
			Rpm2docservVersion: rpm2docservVersion,
			Breadcrumbs: breadcrumbs{
				{"", productName},
			},
			PkgDirs:        pkgdirs,
			SrcPkgDirs:     srcpkgdirs,
			ProductName:    productName,
		        Products:       productList,
		})
	}); err != nil {
		return err
	}

	return nil
}
