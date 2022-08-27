package main

import (
	"sort"
	"strings"

	"github.com/thkukuk/rpm2docserv/pkg/rpm"
)

var manPrefix = "/usr/share/man/"
var gzSuffix = ".gz"


// go through all RPMs, get the filelist, and store the filename of
// all manual pages found in an RPM
func getContents(suite string, pkgs []*pkgEntry) (error) {

	for _, pkg := range pkgs {

		filelist, err := rpm.GetRPMFilelist (pkg.filename)
		if err != nil {
			return err
		}

		for _, filename := range filelist {

			if strings.HasPrefix(filename, manPrefix) && strings.HasSuffix(filename, gzSuffix){
				pkg.manpageList = append(pkg.manpageList, filename)
			}
		}
		if  len(pkg.manpageList) > 0 {
			// sort by lenght of string means, ../man/fr/man?/... will come
			// before ../man/fr.UTF-8/man?/...
			// Same for fr.ISO8859-1
			// The code is not designed to handle different encodings, doesn't
			// make any sense and is not needed.
			sort.Slice(pkg.manpageList, func(i, j int) bool {
				return len(pkg.manpageList[i]) < len(pkg.manpageList[j])
			})
		}
	}

	return nil
}

func getAllContents(suite string, pkgs []*pkgEntry) (error) {
	// XXX Loop over all suites...

	err := getContents(suite, pkgs)
	if err != nil {
		return err
	}

	return nil
}
