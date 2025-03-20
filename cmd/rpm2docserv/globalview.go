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

type stats struct {
	TotalNumberPkgs   uint64
	PackagesExtracted uint64
	ManpagesRendered  uint64
	ManpageBytes      uint64
	HTMLBytes         uint64
	IndexBytes        uint64
}

type globalView struct {
	// pkgs contains all binary packages with manual pages
	pkgs []*manpage.PkgMeta

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

type byProductPkgVer []*manpage.PkgMeta
func (p byProductPkgVer) Len() int      { return len(p) }
func (p byProductPkgVer) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p byProductPkgVer) Less(i, j int) bool {
	if p[i].Product != p[j].Product {
		// if the product is different, sort according to product name
		orderi, oki := sortOrder[p[i].Product]
		orderj, okj := sortOrder[p[j].Product]
		if !oki || !okj {
			// if we have a known product, prefer that over the unknown one
			if oki && !okj {
				return true
			}
			if okj && !oki {
				return false
			}
			return p[i].Product < p[j].Product
		}
		return orderi < orderj
	}
	if p[i].Binarypkg == p[j].Binarypkg {
		// Higher versions should come before lower ones, so higher is less
		return p[j].Version.LessThan(p[i].Version)
	}
	return p[i].Binarypkg < p[j].Binarypkg
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

var manPrefix = "/usr/share/man/"
var gzSuffix = ".gz"

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

// Get the filelist of an RPM and store the filename of all manual pages
// found in that RPM
func getManpageList(fn string) ([]string, error) {
	var manpageList []string

	filelist, err := rpm.GetRPMFilelist (fn)
	if err != nil {
		return nil, err
	}

	for _, filename := range filelist {
		if strings.HasPrefix(filename, manPrefix) && strings.HasSuffix(filename, gzSuffix){
			manpageList = append(manpageList, filename)
		}
	}
	if  len(manpageList) > 0 {
		// sort by lenght of string means, ../man/fr/man?/... will come
		// before ../man/fr.UTF-8/man?/...
		// Same for fr.ISO8859-1
		// The code is not designed to handle different encodings, doesn't
		// make any sense and is not needed.
		sort.Slice(manpageList, func(j, k int) bool {
			return len(manpageList[j]) < len(manpageList[k])
		})
	}
	return manpageList, nil
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
						res.stats.TotalNumberPkgs++
						rpmname := filepath.Base(path)
						binarypkg, rpmversion, rpmrelease, _, sourcepkg, err := rpm.GetRPMHeader(path)
						if err != nil {
							log.Printf("Ignoring %q: %v\n", rpmname, err)
							return nil
						}

						manpageList, err := getManpageList(path)
						if err != nil {
							log.Printf("Ignoring %q: %v\n", path, err)
							return nil
						}
						if len(manpageList) == 0 {
							return nil
						}

						// Add RPM to package list
						pkg := new(manpage.PkgMeta)
						pkg.Sourcepkg, _, _, _, err = rpm.SplitRPMname(sourcepkg) // sourcepkg
						pkg.Product = product.Name
						pkg.Filename = path
						pkg.ManpageList = manpageList
						pkg.Binarypkg = binarypkg
						pkg.Version = version.NewVersion(rpmversion + "-" + rpmrelease)

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
		key := pkg.Product + "/" + pkg.Binarypkg
		if _, exists := latestVersion[key]; !exists {
			latestVersion[key] = pkg
		}
	}

	knownIssues := make(map[string][]error)

	// Build a global view of all the manpages (required for cross-referencing).
	for _, pkg := range res.pkgs {
		if len(pkg.ManpageList) == 0 {
			continue
		}

		key := pkg.Product + "/" + pkg.Binarypkg
		for _, f := range pkg.ManpageList {
			if err := markPresent(latestVersion, res.xref, strings.TrimPrefix(f, manPrefix), key); err != nil {
				knownIssues[key] = append(knownIssues[key], err)
			}
		}
	}

	for key, errors := range knownIssues {
		log.Printf("package %q has errors: %v", key, errors)
	}

	return res, nil
}
