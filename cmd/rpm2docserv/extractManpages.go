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

func extractManpages(cacheDir string, servingDir string, suite string, gv globalView) (error) {

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
				return fmt.Errorf("Trying to interpret path %q: %v", f, err)
			}

			targetdir := filepath.Join(servingDir, p.suite, p.binarypkg)

			err = os.MkdirAll(targetdir, 0755)
			if err != nil {
				return fmt.Errorf("Cannot create target dir %q: %v", targetdir, err)
			}

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
					srcf = link
				} else {
					log.Printf("Ignoring dangling symlink %q\n", f)
					continue
				}
			}

			dstf := filepath.Join(targetdir, m.Name + "." + m.Section + "." + m.Language + ".gz")
			err = os.Link(srcf, dstf)
			if err != nil {
				return fmt.Errorf("Cannot hardlink %q to %q: %v", srcf, dstf, err)
			}
		}

		atomic.AddUint64(&gv.stats.PackagesExtracted, 1)
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
