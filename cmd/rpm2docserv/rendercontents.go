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

func renderContents(dest, suite string, bins []string) error {
	sort.Strings(bins)

	if err := write.Atomically(dest, true, func(w io.Writer) error {
		return contentsTmpl.Execute(w, struct {
			Title          string
			Rpm2docservVersion string
			Breadcrumbs    breadcrumbs
			FooterExtra    string
			Bins           []string
			Suite          string
			Meta           *manpage.Meta
			HrefLangs      []*manpage.Meta
		}{
			Title:          fmt.Sprintf("Contents"),
			Rpm2docservVersion: rpm2docservVersion,
			Breadcrumbs: breadcrumbs{
				{fmt.Sprintf("/%s/index.html", suite), suite},
				{"", "Contents"},
			},
			Bins:  bins,
			Suite: suite,
		})
	}); err != nil {
		return err
	}

	return nil
}
