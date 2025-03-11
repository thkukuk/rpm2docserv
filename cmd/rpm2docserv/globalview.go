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
	suite      string
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

	// Suites is the list of Product Names
        suites map[string]bool

        // idxSuites maps codename, suite and command-line argument to suite (as in
        // suites).
	// e.g. map[MicroOS:Tumbleweed Tumbleweed:Tumbleweed]
        idxSuites map[string]string

	// xref maps from manpage.Meta.Name (e.g. “w3m” or “systemd.service”) to
	// the corresponding manpage.Meta.
	xref map[string][]*manpage.Meta

	stats *stats
	start time.Time
}

type bySuitePkgVer []*pkgEntry
func (p bySuitePkgVer) Len() int      { return len(p) }
func (p bySuitePkgVer) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p bySuitePkgVer) Less(i, j int) bool {
	if p[i].suite != p[j].suite {
		// if the suite is different, sort according to product name
		orderi, oki := sortOrder[p[i].suite]
		orderj, okj := sortOrder[p[j].suite]
		if !oki || !okj {
			// if we have a known suite, prefer that over the unknown one
			if oki && !okj {
				return true
			}
			if okj && !oki {
				return false
			}
			return p[i].suite < p[j].suite
		}
		return orderi < orderj
	}
	if p[i].binarypkg == p[j].binarypkg {
		// Higher versions should come before lower ones, so higher is less
		return p[j].version.LessThan(p[i].version)
	}
	return p[i].binarypkg < p[j].binarypkg
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
func buildGlobalView(suites []Suites, start time.Time) (globalView, error) {
	var stats stats
	res := globalView{
		suites:        make(map[string]bool, len(suites)),
		idxSuites:     make(map[string]string, len(suites)),
		xref:          make(map[string][]*manpage.Meta),
		stats:         &stats,
		start:         start,
	}

	for _, suite := range suites {

		res.suites[suite.Name] = true
		res.idxSuites[suite.Name] = suite.Name
		for _, alias := range suite.Alias {
			res.idxSuites[alias] = suite.Name
		}

		// Walk recursivly through the full cache directory, search all
		// RPMs and store the meta data for them.
		for i := range suite.Cache {
			if *verbose {
				log.Printf("Read %q from %q...", suite.Cache[i], suite.Name)
			}
			err := filepath.WalkDir(suite.Cache[i],
				func(path string, di fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if strings.HasSuffix(path, ".rpm") {
						// Add RPM to package list
						pkg := new(pkgEntry)
						pkg.suite = suite.Name
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
				return res, fmt.Errorf("WalkDir(%q): %v", suite.Cache[i], err)
			}
		}
	}

	// sort the package list, so that packages with a higher version comes first
	sort.Stable(bySuitePkgVer(res.pkgs))

	// build an index with the latest version of a package,
	// ignoring all lower versions of the same package
	latestVersion := make(map[string]*manpage.PkgMeta)
	for _, pkg := range res.pkgs {
		key := pkg.suite + "/" + pkg.binarypkg
		if _, exists := latestVersion[key]; !exists {
			latestVersion[key] = &manpage.PkgMeta{
				Filename: pkg.filename,
				Sourcepkg: pkg.source,
				Binarypkg: pkg.binarypkg,
				Version: pkg.version,
				Product: pkg.suite,
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

		key := p.suite + "/" + p.binarypkg
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
