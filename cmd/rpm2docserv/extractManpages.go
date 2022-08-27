// Copyright 2022 Thorsten Kukuk
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/thkukuk/rpm2docserv/pkg/manpage"
	"github.com/thkukuk/rpm2docserv/pkg/rpm"
)

type manLinks struct {
	rpmfile string
	binarypkg string
	source string
	target string
}

func getUpdateAlternatives(ml manLinks) (string, error) {

	scripts, err := rpm.GetRPMScripts(ml.rpmfile)
	if err != nil {
		return "", err
	}

	for i, line := range scripts {
		pos := strings.Index(line, ml.source)
		if pos >= 0 {
			str := line[pos+len(ml.source):]
			// update-alternatives format normally is:
			// string1 string2 string3, so in worst case:
			// string1 \
			// string2 \
			// string3
			// with this we should have all important entries
			// in one line and can split them in words
			if str[len(str)-1:] == "\\" {
				str = str[:len(str)-1] + scripts[i+1]
				if str[len(str)-1:] == "\\" {
					str = str[:len(str)-1] + scripts[i+2]
				}
			}
			words := strings.Fields(str)
			if len(words) < 2 {
				return "", errors.New("Error: cannot parse update-alternatives entry for " + ml.source)
			}

			m, err :=  manpage.FromManPath(strings.TrimPrefix(words[1], manPrefix), nil)
			if err != nil {
				return "", errors.New("Error: cannot parse " + words[1])
			}

			return m.Name + "." + m.Section + "." + m.Language + ".gz", nil
		}
	}

	return "", errors.New("Error: " + ml.source + " not found in RPM scripts")
}

func extractManpages(cacheDir string, servingDir string, suite string, gv globalView) (error) {

	var missing []*manLinks
	var updateAlternatives []*manLinks

	for _, p := range gv.pkgs {
		if p.suite != suite {
			continue
		}
		if len(p.manpageList) == 0 {
			continue
		}

		tmpdir, err := ioutil.TempDir(servingDir, "tmp-unrpm-")
		if err != nil {
			log.Fatal(err)
		}

		// XXX Error handling and move to rpm packge als rpm.Unpack
		rpm2cpio := exec.Command("rpm2cpio", p.filename)
		cpio := exec.Command("cpio", "-D", tmpdir,
			"--extract",
			"--unconditional",
			"--preserve-modification-time",
			"--make-directories")

		cpio.Stdin, _ = rpm2cpio.StdoutPipe()
		cpio.Stdout = os.Stdout
		_ = cpio.Start()
		_ = rpm2cpio.Run()
		_ = cpio.Wait()


		for _, f := range p.manpageList {
			m, err :=  manpage.FromManPath(strings.TrimPrefix(f, manPrefix), nil)
			if err != nil {
				// not well formated manual page, already reported, ignore it
				continue
			}

			targetdir := filepath.Join(servingDir, p.suite, p.binarypkg)

			err = os.MkdirAll(targetdir, 0755)
			if err != nil {
				return fmt.Errorf("Cannot create target dir %q: %v", targetdir, err)
			}

			dstf := filepath.Join(targetdir, m.Name + "." + m.Section + "." + m.Language + ".gz")

			// check if the source file (manual page) is a symlink. If yes, hardlink the
			// file the symlink points to as target file with the old name
			srcf := filepath.Join(tmpdir, f)
			fileInfo, err := os.Lstat(srcf)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					// %ghost file, most likely update-alternatives...
					updateAlternatives = append (updateAlternatives, &manLinks{
						rpmfile: p.filename,
						binarypkg: p.binarypkg,
						source: f,
						target: dstf,
					})
				} else {
					log.Printf("Error in lstat (%s): %v", p.binarypkg, err)
				}
				continue
			}
			if fileInfo.Mode()&os.ModeSymlink == os.ModeSymlink {
				link, _ := filepath.EvalSymlinks(srcf)
				// Check that we have no dangling symlink and it
				// does not point outside our tmpdir
				if len(link) > 0 && strings.HasPrefix(link, tmpdir) {
					srcf = link
				} else {
					// Most likely the source file is in another RPM or update-alternatives,
					// so save this and try it later again.
					link, err = os.Readlink(srcf)
					if err != nil {
						// ignore this manual page
						log.Printf("Error in Readlink(%q) from %q: %v", srcf, p.binarypkg, err)
						continue
					}

					if strings.HasPrefix(link, "/etc/alternatives/") {
						updateAlternatives = append (updateAlternatives, &manLinks{
							rpmfile: p.filename,
							binarypkg: p.binarypkg,
							source: f,
							target: dstf,
						})
					} else {
						missing = append (missing, &manLinks{
							binarypkg: p.binarypkg,
							source: filepath.Join(filepath.Dir(f),link),
							target: dstf,
						})
					}
					continue
				}
			}

			err = os.Link(srcf, dstf)
			if err != nil {
				log.Printf("Cannot hardlink %q (%s): %v", srcf, p.binarypkg, err)
				continue
			}
		}

		atomic.AddUint64(&gv.stats.PackagesExtracted, 1)
		os.RemoveAll(tmpdir)
	}

	for _, s := range missing {
		m, err :=  manpage.FromManPath(strings.TrimPrefix(s.source, manPrefix), nil)
		if err != nil {
			log.Printf("Error with dangling symlink (%s): src=%q, dst=%q, err=%v\n", s.binarypkg, s.source, s.target, err)
			continue
		}

		found := false
		x := gv.xref[m.Name]
		for _, y := range x {
			if suite == y.Package.Suite {
				srcf := filepath.Join(servingDir, y.ServingPath() + ".gz")
				err = os.Link(srcf, s.target)
				if err != nil {
					log.Printf("Cannot hardlink %q: %v", srcf, err)
					continue
				}
				found = true
				break
			}
		}
		if !found {
			log.Printf("Dangling symlink (%s): src=%q, dst=%q\n", s.binarypkg, s.source, s.target)
			continue
		}
	}

	for _, s := range updateAlternatives {
		source, err := getUpdateAlternatives(*s)
		if err != nil {
			log.Printf("Error parsing update-alternatives (%s): %q -> %q: %v\n", s.binarypkg, s.source, s.target, err)
			continue
		}

		srcf := filepath.Join(servingDir, suite, s.binarypkg, source)
		err = os.Link(srcf, s.target)
		if err != nil {
			log.Printf("Cannot hardlink (%s): %q -> %q: %v\n", s.binarypkg, srcf, s.target, err)
			continue
		}
	}
	return nil
}

func extractManpagesAll(cacheDir string, servingDir string, gv globalView) (error) {
	for suite := range gv.suites {
		// Cleanup directory for suite
		suitedir := filepath.Join(servingDir, suite)
		// XXX error handling!
		os.RemoveAll(suitedir)
		os.MkdirAll(suitedir, 0755)

		err := extractManpages(cacheDir, servingDir, suite, gv)
		if err != nil {
			return err
		}
	}
	return nil
}
