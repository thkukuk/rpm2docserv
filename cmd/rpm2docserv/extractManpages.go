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
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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


var (
	// list of packages, from which we know that the same
	// file will be hardlinked several times and we can ignore them
	// XXX should be part of a yaml config file...
	// XXX define if prefix or whole string...
	// this list must be strictly alpabetical sorted!
	// inn and mininews are conflicting packages build from the same source, so identical manpages
	// python3* uses update-alternatives for the identical manual pages, only build for different python versions
	extractErrorWhitelist = []string{"inn", "mininews", "python3"};
	// qelectrotech ships french manual pages for different locales, we only need one
	linkErrorWhitelist = []string{"qelectrotech"};
)

func isWhitelisted(pkg string, whitelist []string) (bool) {
	i := sort.Search(len(whitelist), func(i int) bool { return pkg <= whitelist[i] })

	if i < len(whitelist) && strings.HasPrefix(pkg, whitelist[i]) {
		return true
	} else {
		return false
	}
}

// Parse RPM postinstall scripts for update-alternatives calls
// and try to find out which manual page it points to by default
func getUpdateAlternatives(filename string, rpmfile string) (string, error) {

	scripts, err := rpm.GetRPMScripts(rpmfile)
	if err != nil {
		return "", fmt.Errorf("rpm.GetRPMScripts(%s) failed: %v\n", rpmfile, err)
	}

	for i, line := range scripts {
		pos := strings.Index(line, filename)
		if pos >= 0 {
			str := line[pos+len(filename):]

			// Remove all '"' around update-alternatives arguments
			str = strings.Replace(str, "\"", "", -1)

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
				return "", errors.New("Error: cannot parse update-alternatives entry for " + filename)
			}

			return words[1], nil
		}
	}

	return "", errors.New("Error: " + filename + " not found in RPM scripts")
}

func getManpageRef(f string, tmpdir string, rpmfile string) (string, error) {

	// check if the source file (manual page) is a symlink. If yes, hardlink the
	// file the symlink points to as target file with the old name
	fileInfo, err := os.Lstat(f)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// %ghost entry, most likely update-alternatives
			dstf, err := getUpdateAlternatives(strings.TrimPrefix(f, tmpdir), rpmfile)
			if err != nil {
				return "", err
			}
			return getManpageRef(filepath.Join(tmpdir, dstf), tmpdir, rpmfile)
		} else {
			return "", err
		}
	}

	if fileInfo.Mode()&os.ModeSymlink == os.ModeSymlink {
		link, _ := filepath.EvalSymlinks(f)
		// Check that we have no dangling symlink and it
		// does not point outside our tmpdir
		if len(link) > 0 && strings.HasPrefix(link, tmpdir) {
			// could point to another link or be a .so reference
			return getManpageRef(link, tmpdir, rpmfile)
		} else {
			// Most likely the source file is in another RPM or update-alternatives,
			link, err = os.Readlink(f)
			if err != nil {
				return "", fmt.Errorf("Error in Readlink(%q): %v", f, err)
			}

			if strings.HasPrefix(link, "/etc/alternatives/") {
				dstf, err := getUpdateAlternatives(strings.TrimPrefix(f, tmpdir), rpmfile)
				if err != nil {
					return "", err
				}
				return getManpageRef(filepath.Join(tmpdir, dstf), tmpdir, rpmfile)
			} else {
				return "", fmt.Errorf("Dangling symlink: %q", f)
			}
		}
	}

	// Handle .so links. Don't open, read and decompress all manpages,
	// the filesize should be smaller than 200 byte if it is only a .so link
	if fileInfo.Size() < 200 {
		// Open the gzip file.
		fh, err := os.Open(f)
		if err != nil {
			return "", fmt.Errorf("Error opening file %q: %v", f, err)
		}
		// Create new reader to decompress gzip.
		reader, err := gzip.NewReader(fh)
		if err != nil {
			fh.Close()
			return "", fmt.Errorf("Error creating gzip.NewReader: %v", err)
		}

		// Empty byte slice.
		result := make([]byte, 300)

		// Read in data.
		count, err := reader.Read(result)
		fh.Close()
		if err != nil && err != io.EOF {
			return "", fmt.Errorf("Error opening gzip compress file %q: %v", f, err)
		}

		str := string(result)[:count]

		// get only the first line, ignore the rest
		pos := strings.Index(str, "\n");
		if pos > 0 {
			str = str[:pos]
		}
		str = strings.TrimSuffix(str, "\n")

		// Make sure it is a .so reference
		if strings.HasPrefix(str, ".so ") {
			var section string

			str = strings.TrimPrefix(str, ".so ")

			pos := strings.Index(str, "/")
			if pos > 0 {
				section = str[:pos]
				str = str[pos+1:]
			} else {
				section = "man" + str[len(str)-1:]
			}

			// We need the prefix of the manpage section,
			// so including a possible lanugage directory.
			// Remove manpage itself and the section
			prefix := filepath.Dir(f)
			prefix = filepath.Dir(prefix)

			soRef := filepath.Join(prefix, section, str + ".gz")

			// Check that the .so reference does not point to itself
			// See [bsc#1202943] as example
			if f == soRef {
				log.Printf("WARNING: %s/%s/%s points to itself!\n", prefix, section, str + ".gz")
				return soRef, nil
			} else {
				return getManpageRef(soRef, tmpdir, rpmfile)
			}
		}
	}
	return f, nil
}

// Unpack a RPM, copy the manual pages in a separate directory together with all other
// manualpages of the source RPM
// We need directories per source RPM to be able to extract conflicting packages.
// We need all manpages from all subpackages of a Source RPM since symlinks and .so
// references are going cross packages.
func unpackRPMs(cacheDir string, tmpdir string, suite string, gv globalView) (error) {

	for _, p := range gv.pkgs {
		if p.suite != suite {
			continue
		}
		if len(p.manpageList) == 0 {
			continue
		}

		unrpmDir := filepath.Join(tmpdir, "unrpm")
		err := os.MkdirAll(unrpmDir, 0755)
		if err != nil {
			os.RemoveAll(unrpmDir)
			return fmt.Errorf("Cannot create directoy %q: %v", unrpmDir, err)
		}

		// XXX rpm packge als rpm.Unpack
		rpm2cpio := exec.Command("rpm2cpio", p.filename)
		cpio := exec.Command("cpio", "-D", unrpmDir,
			"--extract",
			"--unconditional",
			"--preserve-modification-time",
			"--make-directories")

		cpio.Stdin, _ = rpm2cpio.StdoutPipe()
		cpio.Stdout = os.Stdout
		err = cpio.Start()
		if err != nil {
			return fmt.Errorf("Error invoking cpio: %v", err)
		}
		err = rpm2cpio.Run()
		if err != nil {
			return fmt.Errorf("Error invoking rpm2cpio: %v", err)
		}
		err = cpio.Wait()
		if err != nil {
			return fmt.Errorf("Error waiting for cpio: %v", err)
		}

		for _, f := range p.manpageList {
			dstf := filepath.Join(tmpdir, p.sourcerpm, f)

			err = os.MkdirAll(filepath.Dir(dstf), 0755)
			if err != nil {
				os.RemoveAll(unrpmDir)
				return fmt.Errorf("Cannot create directoy %q: %v", filepath.Dir(dstf), err)
			}

			srcf := filepath.Join(unrpmDir, f)

			err = os.Link(srcf, dstf)
			if err != nil && !errors.Is(err, os.ErrNotExist) && !isWhitelisted(p.binarypkg, extractErrorWhitelist) {
				log.Printf("Cannot hardlink %q (%s): %v", srcf, p.binarypkg, err)
				continue
			}
		}
		os.RemoveAll(unrpmDir)
	}

	return nil
}

func extractManpages(cacheDir string, servingDir string, suite string, gv globalView) (error) {

	tmpdir, err := ioutil.TempDir(servingDir, "collect-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	err = unpackRPMs(cacheDir, tmpdir, suite, gv)
	if err != nil {
		return err
	}

	for _, p := range gv.pkgs {
		if p.suite != suite {
			continue
		}
		if len(p.manpageList) == 0 {
			continue
		}

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

			srcf, err := getManpageRef(filepath.Join(tmpdir, p.sourcerpm, f), filepath.Join(tmpdir, p.sourcerpm), p.filename)
			if err != nil {
				log.Printf("Error in finding manpage (%s): %v", p.binarypkg, err)
				continue
			}

			err = os.Link(srcf, dstf)
			if err != nil && !isWhitelisted(p.binarypkg, linkErrorWhitelist){
				log.Printf("Cannot hardlink %q (%s): %v", srcf, p.binarypkg, err)
				continue
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
