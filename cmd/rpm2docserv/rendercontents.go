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

var contentsTmpl = mustParseContentsTmpl()

func mustParseContentsTmpl() *template.Template {
	return template.Must(template.Must(commonTmpls.Clone()).New("contents").Parse(bundled.Asset("contents.tmpl")))
}

func renderProductContents(dest, productName string, bins []string, gv globalView) error {

	if len(bins) == 0 {
		return nil
	}
	sort.Strings(bins)

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
			Bins           []string
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
			Bins:           bins,
			ProductName:    productName,
		        Products:       productList,
		})
	}); err != nil {
		return err
	}

	return nil
}
