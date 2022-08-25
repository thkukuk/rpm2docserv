package main

import (
	"strings"

	"github.com/thkukuk/rpm2docserv/pkg/rpm"
)

type contentEntry struct {
	suite     string
	arch      string
	binarypkg string
	filename  string
}

var manPrefix = "/usr/share/man/"
var gzSuffix = ".gz"


// go through all RPMs, get the filelist, and store the filename of
// all manual pages found in an RPM
func getContents(suite string, pkgs []*pkgEntry) ([]*contentEntry, error) {

	var entries []*contentEntry
	for _, pkg := range pkgs {

		filelist, err := rpm.GetRPMFilelist (pkg.filename)
		if err != nil {
			return nil, err
		}

		for _, filename := range filelist {

			if strings.HasPrefix(filename, manPrefix) && strings.HasSuffix(filename, gzSuffix){
				entries = append(entries, &contentEntry{
					suite:     suite,
					arch:      pkg.arch,
					binarypkg: pkg.binarypkg,
					filename:  strings.TrimPrefix(filename, manPrefix),
				})

				// if pkg.manpageList == nil {
				//	pkg.manpageList = make([]*string, 0, 1)
				//}
				pkg.manpageList = append(pkg.manpageList, filename)
			}
		}
	}

	return entries, nil
}

func getAllContents(suite string, pkgs []*pkgEntry) ([]*contentEntry, error) {
	results, err := getContents(suite, pkgs)
	if err != nil {
		return nil, err
	}
	return results, nil
}
