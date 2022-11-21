// generate sitemap for docserv directroy used by search engines
package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/thkukuk/rpm2docserv/pkg/sitemap"
	"github.com/thkukuk/rpm2docserv/pkg/write"
)

var (
	baseURL = flag.String("base-url",
		"",
                "Base URL (without trailing slash) to the site.")

        servingDir = flag.String("serving-dir",
                "/srv/docserv",
                "Directory in which to place the manpages which should be served")

        verbose = flag.Bool("verbose",
                false,
                "Print additional status messages")

	showVersion = flag.Bool("version",
                false,
                "Show version and exit")
)

// use go build -ldflags "-X main.rpm2docservVersion=<version>" to set the version
var rpm2docservVersion = "HEAD"

func main() {
	flag.Parse()

	if *showVersion {
                fmt.Printf("docserv-sitemap %s\n", rpm2docservVersion)
		return
        }

	if len(*baseURL) == 0 {
		log.Fatal("Usage: docserv-sitemap --base-url=<URL> [--serving-dir=<dir>]")
	}

	log.Printf("docserv sitemap generation for %q", *servingDir)

	err := walkDirs(*servingDir, *baseURL)
	if err != nil {
		log.Fatal(err)
	}
}

func walkDirs(dir string, baseURL string) error {
	sitemaps := make(map[string]time.Time)

	suitedirs, err := ioutil.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("Reading %v failed: %v", dir, err)
	}
	for _, sfi := range suitedirs {
		if !sfi.IsDir() {
			continue
		}

		if *verbose {
			log.Printf("Searching in \"%v\"...", sfi.Name())
		}

		fn := filepath.Join(*servingDir, sfi.Name())
		bins, err := os.Open(fn)
		if err != nil {
			return fmt.Errorf("Cannot open %v: %v", fn, err)
		}
		defer bins.Close()

		// openSUSE Tumbleweed has ~11000 package entries, 20000 should
		// be good enough as start
		sitemapEntries := make(map[string]time.Time, 20000)

		for {
			if *verbose {
				log.Print("Calling Readdirnames...")
			}
			names, err := bins.Readdirnames(0)
			if err != nil {
				if err == io.EOF {
					break
				} else {
					return fmt.Errorf ("Readdirnames failed: %v", err)
				}
			}
			if *verbose {
				log.Printf("Readdirnames found %d entries...", len(names))
			}

			if len(names) == 0 {
				break
			}

			for _, bfn := range names {
				if bfn == "sourcesWithManpages.txt.gz" ||
					bfn == "index.html.gz" ||
					bfn == "sitemap.xml.gz" ||
					bfn == ".nobackup" {
					continue
				}

				fn := filepath.Join(dir, sfi.Name(), bfn)
				fi, err := os.Stat(fn)
				if err != nil {
					return fmt.Errorf("Stat(%v) failed: %v", fn, err)
				}

				if !fi.ModTime().IsZero() {
					sitemapEntries[bfn] = fi.ModTime()
				}
			}
		}
		bins.Close()

		sitemapPath := filepath.Join(dir, sfi.Name(), "sitemap.xml.gz")
		escapedUrlPath := &url.URL{Path: sfi.Name()}
		if err := write.Atomically(sitemapPath, true, func(w io.Writer) error {
			return sitemap.WriteTo(w, baseURL+"/" + escapedUrlPath.String(), sitemapEntries)
		}); err != nil {
			return fmt.Errorf("Write sitemap for %v failed: %v", sfi.Name(), err)
		}
		st, err := os.Stat(sitemapPath)
		if err == nil {
			sitemaps[escapedUrlPath.String()] = st.ModTime()
		}
	}
	return write.Atomically(filepath.Join(dir, "sitemapindex.xml.gz"), true, func(w io.Writer) error {
		return sitemap.WriteIndexTo(w, baseURL, sitemaps)
	})
}
