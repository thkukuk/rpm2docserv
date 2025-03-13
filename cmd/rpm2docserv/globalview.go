package main

import (
	"fmt"
	"log"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/thkukuk/rpm2docserv/pkg/manpage"
	"github.com/thkukuk/rpm2docserv/pkg/rpm"

	"github.com/knqyf263/go-rpm-version"
)

type pkgEntry struct {
        source     string
	sourcerpm  string
	product    string
        binarypkg  string
        arch       string
        filename   string
        version    version.Version
	manpageList []string
}

type stats struct {
	PackagesExtracted uint64
	ManpagesRendered  uint64
	ManpageBytes      uint64
	HTMLBytes         uint64
	IndexBytes        uint64
}

type globalView struct {
	// pkgs contains all binary packages we know of.
	pkgs []*pkgEntry

	// list of product mames for quick check of existence
        products map[string]bool

	// sorted list of product names
	productList []string

        // productMapping maps codename and products
	// e.g. map[MicroOS:Tumbleweed Tumbleweed:Tumbleweed]
        productMapping map[string]string

	// xref maps from manpage.Meta.Name (e.g. “w3m” or “systemd.service”) to
	// the corresponding manpage.Meta.
	xref map[string][]*manpage.Meta

	stats *stats
	start time.Time
}

type byProductPkgVer []*pkgEntry
func (p byProductPkgVer) Len() int      { return len(p) }
func (p byProductPkgVer) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p byProductPkgVer) Less(i, j int) bool {
	if p[i].product != p[j].product {
		// if the product is different, sort according to product name
		orderi, oki := sortOrder[p[i].product]
		orderj, okj := sortOrder[p[j].product]
		if !oki || !okj {
			// if we have a known product, prefer that over the unknown one
			if oki && !okj {
				return true
			}
			if okj && !oki {
				return false
			}
			return p[i].product < p[j].product
		}
		return orderi < orderj
	}
	if p[i].binarypkg == p[j].binarypkg {
		// Higher versions should come before lower ones, so higher is less
		return p[j].version.LessThan(p[i].version)
	}
	return p[i].binarypkg < p[j].binarypkg
}

type byProductStr []string
func (p byProductStr) Len() int      { return len(p) }
func (p byProductStr) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p byProductStr) Less(i, j int) bool {
	orderi, oki := sortOrder[p[i]]
	orderj, okj := sortOrder[p[j]]
	if !oki || !okj {
		// if we have know a suite, prefer that over the unknown one
		if oki && !okj {
			return true
		}
		if okj && !oki {
			return false
		}
		return p[i] < p[j]
	}
	return orderi < orderj
}

func markPresent(latestVersion map[string]*manpage.PkgMeta, xref map[string][]*manpage.Meta, filename string, key string) error {
        if _, ok := latestVersion[key]; !ok {
                return fmt.Errorf("Could not determine latest version")
        }
        m, err := manpage.FromManPath(strings.TrimPrefix(filename, manPrefix), latestVersion[key])
        if err != nil {
                return fmt.Errorf("Trying to interpret path %q: %v", filename, err)
        }
        // NOTE(stapelberg): this additional verification step
        // is necessary because manpages such as the French
        // manpage for qelectrotech(1) are present in multiple
        // encodings. manpageFromManPath ignores encodings, so
        // if we didn’t filter, we would end up with what
        // looks like duplicates.
        present := false
        for _, x := range xref[m.Name] {
                if x.ServingPath() == m.ServingPath() {
                        present = true
                        break
                }
        }
        if !present {
                xref[m.Name] = append(xref[m.Name], m)
        }
        return nil
}

// go through the cache directory, find all RPMs and build a pkg entry for it
func buildGlobalView(products []Product, start time.Time) (globalView, error) {
	var stats stats
	res := globalView{
		products:       make(map[string]bool, len(products)),
		productList:    make([]string, 0, len(products)),
		productMapping: make(map[string]string, len(products)),
		xref:           make(map[string][]*manpage.Meta),
		stats:          &stats,
		start:          start,
	}

	for _, product := range products {

		res.productList = append(res.productList, product.Name)
		res.products[product.Name] = true
		res.productMapping[product.Name] = product.Name
		for _, alias := range product.Alias {
			res.productMapping[alias] = product.Name
		}

		// Walk recursivly through the full cache directory, search all
		// RPMs and store the meta data for them.
		for i := range product.Cache {
			if *verbose {
				log.Printf("Read %q from %q...", product.Cache[i], product.Name)
			}
			err := filepath.WalkDir(product.Cache[i],
				func(path string, di fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if strings.HasSuffix(path, ".rpm") {
						// Add RPM to package list
						pkg := new(pkgEntry)
						pkg.product = product.Name
						pkg.filename = path

						var rpmversion, rpmrelease string
						rpmname := filepath.Base(path)
						pkg.binarypkg, rpmversion, rpmrelease, pkg.arch, err = rpm.SplitRPMname2(rpmname, path)
						if err != nil {
							log.Printf("Ignoring %q: %v\n", rpmname, err)
							return nil
						}
						pkg.version = version.NewVersion(rpmversion + "-" + rpmrelease)

						pkg.sourcerpm, err = rpm.GetSourceRPMName(path)
						if err != nil {
							return err
						}
						// We don't need the version and rest of the source RPM name
						pkg.source, _, _, _, err = rpm.SplitRPMname(pkg.sourcerpm)

						res.pkgs = append (res.pkgs, pkg)
					}
					return nil
				})
			if err != nil {
				return res, fmt.Errorf("WalkDir(%q): %v", product.Cache[i], err)
			}
		}
	}

	// sort product list according to product sort order.
	sort.Stable(byProductStr(res.productList))

	// sort the package list, so that packages with a higher version comes first
	sort.Stable(byProductPkgVer(res.pkgs))

	// build an index with the latest version of a package,
	// ignoring all lower versions of the same package
	latestVersion := make(map[string]*manpage.PkgMeta)
	for _, pkg := range res.pkgs {
		key := pkg.product + "/" + pkg.binarypkg
		if _, exists := latestVersion[key]; !exists {
			latestVersion[key] = &manpage.PkgMeta{
				Filename: pkg.filename,
				Sourcepkg: pkg.source,
				Binarypkg: pkg.binarypkg,
				Version: pkg.version,
				Product: pkg.product,
			}
		}
	}

	err := getAllContents(res.pkgs)
	if err != nil {
		return res, err
	}

	knownIssues := make(map[string][]error)

	// Build a global view of all the manpages (required for cross-referencing).
	for _, p := range res.pkgs {
		if len(p.manpageList) == 0 {
			continue
		}

		key := p.product + "/" + p.binarypkg
		for _, f := range p.manpageList {
			if err := markPresent(latestVersion, res.xref, strings.TrimPrefix(f, manPrefix), key); err != nil {
				knownIssues[key] = append(knownIssues[key], err)
			}
		}
	}

	for key, errors := range knownIssues {
		log.Printf("package %q has errors: %v", key, errors)
	}

	return res, err
}
