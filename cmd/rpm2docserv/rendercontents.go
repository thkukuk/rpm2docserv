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

func renderContents(dest, suite string, bins []string, gv globalView) error {
	sort.Strings(bins)

	suites := make([]string, 0, len(gv.suites))
	for suite := range gv.suites {
		suites = append(suites, suite)
	}
	sort.Stable(bySuiteStr(suites))

	if err := write.Atomically(dest, true, func(w io.Writer) error {
		return contentsTmpl.Execute(w, struct {
			Title          string
			ProductName    string
			ProductUrl     string
			LogoUrl        string
			Rpm2docservVersion string
			Breadcrumbs    breadcrumbs
			FooterExtra    string
			Bins           []string
			Suite          string
			Suites         []string
			Meta           *manpage.Meta
			HrefLangs      []*manpage.Meta
		}{
			Title:          fmt.Sprintf("Manpages of %s", suite),
			ProductName:    productName,
			ProductUrl:     productUrl,
			LogoUrl:        logoUrl,
			Rpm2docservVersion: rpm2docservVersion,
			Breadcrumbs: breadcrumbs{
				{"", suite},
			},
			Bins:           bins,
			Suite:          suite,
		        Suites:         suites,
		})
	}); err != nil {
		return err
	}

	return nil
}
