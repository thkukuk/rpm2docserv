package main

import (
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"syscall"

	"github.com/thkukuk/rpm2docserv/pkg/commontmpl"
	"github.com/thkukuk/rpm2docserv/pkg/convert"
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

type renderingMode int

const (
	regularFiles renderingMode = iota
	symlinks
)

// listManpages lists all files in dir (non-recursively) and returns a map from
// filename (within dir) to *manpage.Meta.
func listManpages(dir string) (map[string]*manpage.Meta, error) {
	manpageByName := make(map[string]*manpage.Meta)

	files, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer files.Close()

	var predictedEOF bool
	for {
		if predictedEOF {
			break
		}

		names, err := files.Readdirnames(2048)
		if err != nil {
			if err == io.EOF {
				break
			} else {
				// We avoid an additional stat syscalls for each
				// binary package directory by just optimistically
				// calling readdir and handling the ENOTDIR error.
				if sce, ok := err.(*os.SyscallError); ok && sce.Err == syscall.ENOTDIR {
					return nil, nil
				}
				return nil, err
			}
		}

		// When len(names) < 2048 the next Readdirnames() call will
		// result in io.EOF and can be skipped to reduce getdents(2)
		// syscalls by half.
		predictedEOF = len(names) < 2048

		for _, fn := range names {
			if !strings.HasSuffix(fn, ".gz") ||
				strings.HasSuffix(fn, ".html.gz") {
				continue
			}
			full := filepath.Join(dir, fn)

			m, err := manpage.FromServingPath(*servingDir, full)
			if err != nil {
				// If we run into this case, our code cannot correctly
				// interpret the result of ServingPath().
				log.Printf("BUG: cannot parse manpage from serving path %q: %v", full, err)
				continue
			}

			manpageByName[fn] = m
		}
	}
	return manpageByName, nil
}

func renderDirectoryIndex(dir string, gv globalView) error {
	manpageByName, err := listManpages(dir)
	if err != nil {
		return err
	}

	if len(manpageByName) == 0 {
		log.Printf("WARNING: empty directory %q, not generating package index", dir)
		return nil
	}

	return renderPkgindex(filepath.Join(dir, "index.html"), manpageByName, gv)
}

// walkManContents walks over all entries in dir and, depending on mode, does:
// 1. send a renderJob for each regular file
// 2. send a renderJob for each symlink
func walkManContents(ctx context.Context, renderChan chan<- renderJob, dir string, mode renderingMode, gv globalView) error {
	// the invariant is: each file ending in .gz must have a corresponding .html.gz file
	// the .html.gz must have a modtime that is >= the modtime of the .gz file

	files, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer files.Close()

	var predictedEOF bool
	for {
		if predictedEOF {
			break
		}

		names, err := files.Readdirnames(2048)
		if err != nil {
			if err == io.EOF {
				break
			} else {
				// We avoid an additional stat syscalls for each
				// binary package directory by just optimistically
				// calling readdir and handling the ENOTDIR error.
				if sce, ok := err.(*os.SyscallError); ok && sce.Err == syscall.ENOTDIR {
					return nil
				}
				return err
			}
		}

		// When len(names) < 2048 the next Readdirnames() call will
		// result in io.EOF and can be skipped to reduce getdents(2)
		// syscalls by half.
		predictedEOF = len(names) < 2048

		for _, fn := range names {
			if !strings.HasSuffix(fn, ".gz") ||
				strings.HasSuffix(fn, ".html.gz") {
				continue
			}
			full := filepath.Join(dir, fn)

			st, err := os.Lstat(full)
			if err != nil {
				continue
			}

			symlink := st.Mode()&os.ModeSymlink != 0

			if !symlink {
				atomic.AddUint64(&gv.stats.ManpageBytes, uint64(st.Size()))
			}

			if mode == regularFiles && symlink ||
				mode == symlinks && !symlink {
				continue
			}

			m, err := manpage.FromServingPath(*servingDir, full)
			if err != nil {
				// If we run into this case, our code cannot correctly
				// interpret the result of ServingPath().
				log.Printf("BUG: cannot parse manpage from serving path %q: %v", full, err)
				continue
			}

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

			var reuse string
			if symlink {
				link, err := os.Readlink(full)
				if err == nil {
					resolved := filepath.Join(dir, link)
					reuse = strings.TrimSuffix(resolved, ".gz") + ".html.gz"
				}
			}

			n := strings.TrimSuffix(fn, ".gz") + ".html.gz"
			select {
				case renderChan <- renderJob{
					dest:     filepath.Join(dir, n),
					src:      full,
					meta:     m,
					versions: versions,
					xref:     gv.xref,
					modTime:  st.ModTime(),
					reuse:    reuse,
				}:
			case <-ctx.Done():
				break
			}
		}
	}

	return nil
}

func walkContents(ctx context.Context, renderChan chan<- renderJob, gv globalView) error {

	suitedirs, err := os.ReadDir(*servingDir)
	if err != nil {
		return err
	}
	for _, sfi := range suitedirs {
		if !sfi.IsDir() {
			continue
		}
		if !gv.suites[sfi.Name()] {
			continue
		}
		bins, err := os.Open(filepath.Join(*servingDir, sfi.Name()))
		if err != nil {
			return err
		}
		defer bins.Close()

		for {
			names, err := bins.Readdirnames(*manwalkConcurrency)
			if err != nil {
				if err == io.EOF {
					break
				} else {
					return err
				}
			}

			var wg errgroup.Group
			for _, bfn := range names {
				if bfn == "sourcesWithManpages.txt.gz" ||
					bfn == "idex.html" ||
					bfn == "index.html.gz" ||
					bfn == "sitemap.xml.gz" ||
					bfn == ".nobackup" {
					continue
				}

				bfn := bfn // copy
				dir := filepath.Join(*servingDir, sfi.Name(), bfn)
				wg.Go(func() error {
					var err error
					// Render all regular files first
					err = walkManContents(ctx, renderChan, dir, regularFiles, gv)
					if err != nil {
						return err
					}

					// then render all symlinks, re-using the rendered fragments
					err = walkManContents(ctx, renderChan, dir, symlinks, gv)
					if err != nil {
						return err
					}

					// and finally render the package index files which need to
					// consider both regular files and symlinks.
					if err := renderDirectoryIndex(dir, gv); err != nil {
						return err
					}

					return nil
				})
			}
			if err := wg.Wait(); err != nil {
				return err
			}
		}
		bins.Close()

	}
	return nil
}

func writeSourceIndex(gv globalView) error {
	// Partition by product for reduced memory usage and better locality of file
	// system access
	for suite := range gv.suites {
		binariesBySource := make(map[string][]string)
		for _, p := range gv.pkgs {
			if p.suite == suite {
				binariesBySource[p.source] = append(binariesBySource[p.source], p.binarypkg)
			}
		}

		for src, binaries := range binariesBySource {
			srcDir := filepath.Join(*servingDir, suite, "src:"+src)

			// Aggregate manpages of all binary packages for this source package
			manpages := make(map[string]*manpage.Meta)
			for _, binary := range binaries {
				m, err := listManpages(filepath.Join(*servingDir, suite, binary))
				if err != nil {
					if os.IsNotExist(err) {
						continue // The package might not contain any manpages.
					}
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
	}
	return nil
}

func renderAll(gv globalView) error {
	log.Printf("Preparing inverted maps")

	eg, ctx := errgroup.WithContext(context.Background())
	renderChan := make(chan renderJob)
	for i := 0; i < *renderConcurrency; i++ {
		eg.Go(func() error {
			converter, err := convert.NewProcess()
			if err != nil {
				return err
			}
			defer converter.Kill()

			// NOTE(stapelberg): gzip’s decompression phase takes the same
			// time, regardless of compression level. Hence, we invest the
			// maximum CPU time once to achieve the best compression.
			gzipw, err := gzip.NewWriterLevel(nil, *gzipLevel)
			if err != nil {
				return err
			}

			for r := range renderChan {
				n, err := rendermanpage(gzipw, converter, r, gv)
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

	if err := walkContents(ctx, renderChan, gv); err != nil {
		return err
	}

	close(renderChan)
	if err := eg.Wait(); err != nil {
		return err
	}

	if err := writeSourceIndex(gv); err != nil {
		return fmt.Errorf("writing source index: %v", err)
	}

	for _, product := range productList {
		if !gv.suites[product] {
			log.Printf("ERROR: %s not known in gv.suites (%q)", product, gv.suites)
			continue
		}

		b_pkgdirs := make(map[string]bool)
		b_srcpkgdirs := make(map[string]bool)
		for _, x := range gv.xref {
			for _, m := range x {
				if (product == m.Package.Suite) {
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

		if err := renderProductContents(filepath.Join(*servingDir, product, "index.html",), product, pkgdirs, srcpkgdirs, gv); err != nil {
			return err
		}
	}

	return nil
}
