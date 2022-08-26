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
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/thkukuk/rpm2docserv/pkg/manpage"
)

type manLinks struct {
	source string
	target string
}

func extractManpages(cacheDir string, servingDir string, suite string, gv globalView) (error) {

	var missing []*manLinks

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
		defer os.RemoveAll(tmpdir)

		// XXX Error handling!
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
				// ignore this manual page
				log.Printf("Error in lstat(%q) from %q: %v", srcf, p.binarypkg, err)
				continue
			}
			if fileInfo.Mode()&os.ModeSymlink == os.ModeSymlink {
				link, _ := filepath.EvalSymlinks(srcf)
				if len(link) > 0 {
					if !strings.HasPrefix(link, tmpdir) {
						// XXX if we have no tmpdir prefix, this is going somewhere else,
						// most likely an update-alternative symlink from the host. Ignore.
						log.Printf("Ignoring symlink pointing outside: %q from %q\n", link, p.binarypkg)
						continue
					} else {
						srcf = link
					}
				} else {
					// Most likely the source file is in another RPM, so save this and
					// try it later again.
					link, err = os.Readlink(srcf)
					if err != nil {
						// ignore this manual page
						log.Printf("Error in Readlink(%q) from %q: %v", srcf, p.binarypkg, err)
						continue
					}

					missing = append (missing, &manLinks{
						source: filepath.Join(filepath.Dir(f),link),
						target: dstf,
					})
					continue
				}
			}

			err = os.Link(srcf, dstf)
			if err != nil {
				log.Printf("Cannot hardlink %q to %q: %v", srcf, dstf, err)
				continue
			}
		}

		atomic.AddUint64(&gv.stats.PackagesExtracted, 1)
	}

	for _, s := range missing {
		m, err :=  manpage.FromManPath(strings.TrimPrefix(s.source, manPrefix), nil)
		if err != nil {
			log.Printf("Error with dangling symlink: src=%q, dst=%q, err=%v\n", s.source, s.target, err)
			continue
		}

		found := false
		x := gv.xref[m.Name]
		for _, y := range x {
			if suite == y.Package.Suite {
				srcf := filepath.Join(servingDir, y.ServingPath() + ".gz")
				err = os.Link(srcf, s.target)
				if err != nil {
					log.Printf("Cannot hardlink %q to %q: %v", srcf, s.target, err)
					continue
				}
				found = true
				break
			}
		}
		if !found {
			log.Printf("Dangling symlink: src=%q, dst=%q\n", s.source, s.target)
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
