package main

import (
	"html/template"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thkukuk/rpm2docserv/pkg/bundled"
	"github.com/thkukuk/rpm2docserv/pkg/manpage"
	"github.com/thkukuk/rpm2docserv/pkg/write"
)

var indexTmpl = mustParseIndexTmpl()

func mustParseIndexTmpl() *template.Template {
	return template.Must(template.Must(commonTmpls.Clone()).New("index").Parse(bundled.Asset("index.tmpl")))
}

var aboutTmpl = mustParseAboutTmpl()

func mustParseAboutTmpl() *template.Template {
	return template.Must(template.Must(commonTmpls.Clone()).New("about").Parse(bundled.Asset("about.tmpl")))
}

func renderAux(destDir string, gv *globalView) error {
	aliases := make([]string, 0, len(gv.productMapping))
	for alias := range gv.productMapping {
		aliases = append(aliases, alias)
	}
	sort.Stable(byProductStr(aliases))

	if err := write.Atomically(filepath.Join(destDir, "index.html"), false, func(w io.Writer) error {
		return indexTmpl.Execute(w, struct {
			Title              string
			ProjectName        string
			ProjectUrl         string
			Aliases            []string
			Products           []string
			Breadcrumbs        breadcrumbs
			FooterExtra        string
			HrefLangs          []*manpage.Meta
			IsOffline          bool
			LogoUrl            string
			Meta               *manpage.Meta
			Rpm2docservVersion string
		}{
			Title:              "",
			ProjectName:        projectName,
			ProjectUrl:         projectUrl,
			Aliases:            aliases,
			LogoUrl:            logoUrl,
			Products:           gv.productList,
			IsOffline:          isOffline,
			Rpm2docservVersion: rpm2docservVersion,
		})
	}); err != nil {
		return err
	}

	if err := write.Atomically(filepath.Join(destDir, "about.html.gz"), true, func(w io.Writer) error {
		return aboutTmpl.Execute(w, struct {
			Title          string
			ProjectName    string
			ProjectUrl     string
			LogoUrl        string
			IsOffline      bool
			Rpm2docservVersion string
			Breadcrumbs    breadcrumbs
			FooterExtra    string
			Meta           *manpage.Meta
			HrefLangs      []*manpage.Meta
			Products       []string
			Aliases        []string
		}{
			Title:          "About",
			ProjectName:    projectName,
			ProjectUrl:     projectUrl,
			LogoUrl:        logoUrl,
			IsOffline:      isOffline,
			Rpm2docservVersion: rpm2docservVersion,
			Products:       gv.productList,
			Aliases:        aliases,
		})
	}); err != nil {
		return err
	}

	for name, content := range bundled.AssetsFiltered(func(fn string) bool {
		return !strings.HasSuffix(fn, ".tmpl") && !strings.HasSuffix(fn, ".css")
	}) {
		if err := write.Atomically(filepath.Join(destDir, filepath.Base(name)), false, func(w io.Writer) error {
			_, err := io.WriteString(w, content)
			return err
		}); err != nil {
			return err
		}
	}

	return nil
}
