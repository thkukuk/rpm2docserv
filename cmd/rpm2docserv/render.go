package main

import (
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/thkukuk/rpm2docserv/pkg/commontmpl"
	"github.com/thkukuk/rpm2docserv/pkg/manpage"
	"golang.org/x/net/context"
	"golang.org/x/sync/errgroup"
)

var (
	manwalkConcurrency = flag.Int("concurrency_manwalk",
		1000, // below the default 1024 open file descriptor limit
		"Concurrency level for walking through binary package man directories (ulimit -n must be higher!)")

	renderConcurrency = flag.Int("concurrency_render",
		5,
		"Concurrency level for rendering manpages using mandoc")

	gzipLevel = flag.Int("gzip",
		9,
		"gzip compression level to use for compressing HTML versions of manpages. defaults to 9 to keep network traffic minimal, but useful to reduce for development/disaster recovery (level 1 results in a 2x speedup!)")

)

type breadcrumb struct {
	Link string
	Text string
}

type breadcrumbs []breadcrumb

func (b breadcrumbs) ToJSON() template.JS {
	type item struct {
		Type string `json:"@type"`
		ID   string `json:"@id"`
		Name string `json:"name"`
	}
	type listItem struct {
		Type     string `json:"@type"`
		Position int    `json:"position"`
		Item     item   `json:"item"`
	}
	type breadcrumbList struct {
		Context  string     `json:"@context"`
		Type     string     `json:"@type"`
		Elements []listItem `json:"itemListElement"`
	}
	l := breadcrumbList{
		Context:  "http://schema.org",
		Type:     "BreadcrumbList",
		Elements: make([]listItem, len(b)),
	}
	for idx, br := range b {
		l.Elements[idx] = listItem{
			Type:     "ListItem",
			Position: idx + 1,
			Item: item{
				Type: "Thing",
				ID:   br.Link,
				Name: br.Text,
			},
		}
	}

	jsonb, err := json.Marshal(l)
	if err != nil {
		log.Fatal(err)
	}

	return template.JS(jsonb)
}

var commonTmpls = commontmpl.MustParseCommonTmpls()

// listManpages lists all files in dir (non-recursively) and returns a map from
// filename (within dir) to *manpage.Meta.
func listManpages(product string, pkg string, gv *globalView) (map[string]*manpage.Meta, error) {
	manpageByName := make(map[string]*manpage.Meta)

	for _, x := range gv.xref {
                for _, m := range x {
			if m.Package.Product == product && m.Package.Binarypkg == pkg {
				manpageByName[m.Name+"."+m.Section+"."+m.Language] = m
			}
		}
	}
	return manpageByName, nil
}

// walkManContents walks over all entries in dir and send a renderJob for each file
func walkManContents(ctx context.Context, renderChan chan<- renderJob, product string, pkg string, gv *globalView) error {

	for _, x := range gv.xref {
                for _, m := range x {
			if m.Package.Product != product || m.Package.Binarypkg != pkg {
				continue
			}

			fn := m.Name+"."+m.Section+"."+m.Language+".gz"
			full := filepath.Join(*servingDir, product, pkg, fn)

			st, err := os.Lstat(full)
			if err != nil {
				continue
			}

			atomic.AddUint64(&gv.stats.ManpageBytes, uint64(st.Size()))

			versions := gv.xref[m.Name]
			// Replace m with its corresponding entry in versions
			// so that rendermanpage() can use pointer equality to
			// efficiently skip entries.
			for _, v := range versions {
				if v.ServingPath() == m.ServingPath() {
					m = v
					break
				}
			}

			n := strings.TrimSuffix(full, ".gz") + ".html.gz"
			select {
				case renderChan <- renderJob{
					dest:     n,
					src:      full,
					meta:     m,
					versions: versions,
					xref:     gv.xref,
					modTime:  st.ModTime(),
				}:
			case <-ctx.Done():
				break
			}
		}
	}

	return nil
}

func walkProductContents(ctx context.Context, renderChan chan<- renderJob, product string, binarypkgs []string, gv *globalView) error {

	var wg errgroup.Group
	for _, pkg := range binarypkgs {

		wg.Go(func() error {
			var err error
			// Render all regular files first
			err = walkManContents(ctx, renderChan, product, pkg, gv)
			if err != nil {
				return err
			}

			// and finally render the package index files
			if err := writeBinaryPkgIndex(product, pkg, gv); err != nil {
				return err
			}

			return nil
		})
	}
	if err := wg.Wait(); err != nil {
		return err
	}

	return nil
}


// This function creates the index.html for product/binarypkg
func writeBinaryPkgIndex(product string, binarypkg string, gv *globalView) error {
	manpageByName, err := listManpages(product, binarypkg, gv)
	if err != nil {
		return err
	}

	if len(manpageByName) == 0 {
		log.Printf("WARNING: empty directory %s/%s/%s, not generating package index",
			*servingDir, product, binarypkg)
		return nil
	}

	return renderPkgIndex(filepath.Join(*servingDir, product, binarypkg, "index.html"), manpageByName, gv)
}

// This function creates the index.html for product/src:package where the
// manpage links point to the manual pages in the binary package directory
func writeSourcePkgIndex(product string, gv *globalView) error {
	// Partition by product for reduced memory usage and better locality of file
	// system access
	binariesBySource := make(map[string][]string)
	for _, p := range gv.pkgs {
		if p.product == product {
			binariesBySource[p.source] = append(binariesBySource[p.source], p.binarypkg)
		}
	}

	for src, binaries := range binariesBySource {
		srcDir := filepath.Join(*servingDir, product, "src:"+src)

		// Aggregate manpages of all binary packages for this source package
		manpages := make(map[string]*manpage.Meta)
		for _, binary := range binaries {
			m, err := listManpages(product, binary, gv)
			if err != nil {
				return err
			}
			for k, v := range m {
				manpages[k] = v
			}
		}
		if len(manpages) == 0 {
			continue // The entire source package does not contain any manpages.
		}

		if err := os.MkdirAll(srcDir, 0755); err != nil {
			return err
		}
		if err := renderSrcPkgIndex(filepath.Join(srcDir, "index.html"), src, manpages, gv); err != nil {
			return err
		}
	}
	return nil
}

func renderAll(gv *globalView) error {
	log.Printf("Preparing inverted maps")

	eg, ctx := errgroup.WithContext(context.Background())
	renderChan := make(chan renderJob)
	for i := 0; i < *renderConcurrency; i++ {
		eg.Go(func() error {
			// NOTE(stapelberg): gzip’s decompression phase takes the same
			// time, regardless of compression level. Hence, we invest the
			// maximum CPU time once to achieve the best compression.
			gzipw, err := gzip.NewWriterLevel(nil, *gzipLevel)
			if err != nil {
				return err
			}

			for r := range renderChan {
				n, err := rendermanpage(gzipw, r, gv)
				if err != nil {
					// rendermanpage writes an error page if rendering
					// failed, any returned error is severe (e.g. file
					// system full) and should lead to termination.
					return err
				}

				atomic.AddUint64(&gv.stats.HTMLBytes, n)
				atomic.AddUint64(&gv.stats.ManpagesRendered, 1)
			}
			return nil
		})
	}

	for _, product := range gv.productList {
		log.Printf("Start rendering %s", product)

		b_pkgdirs := make(map[string]bool)
		b_srcpkgdirs := make(map[string]bool)
		for _, x := range gv.xref {
			for _, m := range x {
				if (product == m.Package.Product) {
					b_pkgdirs[m.Package.Binarypkg] = true
					b_srcpkgdirs["src:" + m.Package.Sourcepkg] = true
				}
			}
		}

		pkgdirs := make([]string, 0, len(b_pkgdirs))
		srcpkgdirs := make([]string, 0, len(b_srcpkgdirs))

		for e := range b_pkgdirs {
			pkgdirs = append(pkgdirs, e)
		}
		for e := range b_srcpkgdirs {
			srcpkgdirs = append(srcpkgdirs, e)
		}

		sort.Strings(pkgdirs)
		sort.Strings(srcpkgdirs)

		if len(pkgdirs) + len(srcpkgdirs) == 0 {
			continue
		}

		if err := walkProductContents(ctx, renderChan, product, pkgdirs, gv); err != nil {
			return err
		}

		if err := writeSourcePkgIndex(product, gv); err != nil {
			return fmt.Errorf("writing source index for %s: %v", product, err)
		}

		if err := renderProductContents(filepath.Join(*servingDir, product, "index.html",), product, pkgdirs, srcpkgdirs, gv); err != nil {
			return err
		}
	}

	close(renderChan)
	if err := eg.Wait(); err != nil {
		return err
	}

	return nil
}
