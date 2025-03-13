package main

import (
	"fmt"
	"html/template"

	"github.com/thkukuk/rpm2docserv/pkg/bundled"
)

var contentsTmpl = mustParseContentsTmpl()

func mustParseContentsTmpl() *template.Template {
	return template.Must(template.Must(commonTmpls.Clone()).New("contents").Parse(bundled.Asset("contents.tmpl")))
}

func renderProductContents(dest, productName string, pkgdirs []string, srcpkgdirs []string, gv *globalView) error {
	if err := renderExec(dest, gv, contentsTmpl, tmplData {
		Title:          fmt.Sprintf("Manpages of %s", productName),
		Breadcrumbs: breadcrumbs{
			{"", productName},
		},
		PkgDirs:        pkgdirs,
		SrcPkgDirs:     srcpkgdirs,
		ProductName:    productName,
	}); err != nil {
		return err
	}

	return nil
}
