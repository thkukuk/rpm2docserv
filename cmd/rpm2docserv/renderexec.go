package main

import (
	"html/template"
	"io"

	"github.com/thkukuk/rpm2docserv/pkg/manpage"
	"github.com/thkukuk/rpm2docserv/pkg/write"
)

type tmplData struct {
	// the following variables are set by us
	ProjectName        string
	ProjectUrl         string
	LogoUrl            string
	IsOffline          bool
	Rpm2docservVersion string
	Products           []string
	// the following variables needs to be set by the caller
	Title              string
	Breadcrumbs        breadcrumbs
	FooterExtra        template.HTML
	ProductName        string
	Meta               *manpage.Meta
	HrefLangs          []*manpage.Meta
	PkgDirs            []string
	SrcPkgDirs         []string
}

func renderExec(dest string, gv *globalView, tmpl *template.Template, data tmplData) error {

	data.ProjectName = projectName
	data.ProjectUrl = projectUrl
	data.LogoUrl = logoUrl
	data.IsOffline = isOffline
	data.Rpm2docservVersion = rpm2docservVersion
	data.Products = gv.productList

        return write.Atomically(dest, false, func(w io.Writer) error {
                return tmpl.Execute(w, data)
        })
}
