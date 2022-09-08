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
func getAllContents(pkgs []*pkgEntry) (error) {

	for i := range pkgs {

		// skip binary packages with a lower version, we only use the manpages
		// from the highest one
		if i > 0 && pkgs[i].suite == pkgs[i-1].suite &&
			pkgs[i].binarypkg == pkgs[i-1].binarypkg {
			continue
		}

		filelist, err := rpm.GetRPMFilelist (pkgs[i].filename)
		if err != nil {
			return err
		}

		for _, filename := range filelist {

			if strings.HasPrefix(filename, manPrefix) && strings.HasSuffix(filename, gzSuffix){
				pkgs[i].manpageList = append(pkgs[i].manpageList, filename)
			}
		}
		if  len(pkgs[i].manpageList) > 0 {
			// sort by lenght of string means, ../man/fr/man?/... will come
			// before ../man/fr.UTF-8/man?/...
			// Same for fr.ISO8859-1
			// The code is not designed to handle different encodings, doesn't
			// make any sense and is not needed.
			sort.Slice(pkgs[i].manpageList, func(j, k int) bool {
				return len(pkgs[i].manpageList[j]) < len(pkgs[i].manpageList[k])
			})
		}
	}

	return nil
}
