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
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

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

	yamlConfig = flag.String("config",
                "",
                "Configuration file in yaml format")

        verbose = flag.Bool("verbose",
                false,
                "Print additional status messages")

	showVersion = flag.Bool("version",
                false,
                "Show version and exit")
)

// use go build -ldflags "-X main.rpm2docservVersion=<version>" to set the version
var rpm2docservVersion = "HEAD"

type Suites struct {
        Name     string   `yaml:"name"`
        Cache    []string `yaml:"cache,omitempty"`
        Packages []string `yaml:"packages,omitempty"`
}

type Config struct {
        ProductName string `yaml:"productname,omitempty"`
        ProductUrl  string `yaml:"producturl,omitempty"`
        LogoUrl     string `yaml:"logourl,omitempty"`
        AssetsDir   string `yaml:"assets,omitempty"`
        ServingDir  string `yaml:"servingdir"`
        IndexPath   string `yaml:"auxindex"`
        Download    string `yaml:"download"`
        IsOffline   bool   `yaml:"offline,omitempty"`
        BaseUrl     string `yaml:"baseurl,omitempty"`
        Products    []Suites `yaml:"products"`
        SortOrder   []string `yaml:"sortorder"`
}

func read_yaml_config(conffile string) (Config, error) {

        var config Config

        file, err := ioutil.ReadFile(conffile)
        if err != nil {
                return config, fmt.Errorf("Cannot read %q: %v", conffile, err)
        }
        err = yaml.Unmarshal(file, &config)
        if err != nil {
                return config, fmt.Errorf("Unmarshal error: %v", err)
        }

        return config, nil
}

func main() {
	flag.Parse()

	if *showVersion {
                fmt.Printf("docserv-sitemap %s\n", rpm2docservVersion)
		return
        }

        if len(*yamlConfig) > 0 {
                config, err := read_yaml_config(*yamlConfig)
                if err != nil {
                        log.Fatal(err)
                }
		if len(config.ServingDir) > 0 {
                        servingDir = &config.ServingDir
                }
		if len(config.BaseUrl) > 0 {
			baseURL = &config.BaseUrl
                }
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

func collectFiles(basedir string, dir string, sitemapEntries map[string]time.Time) error {

	fn := filepath.Join(basedir, dir)
	entries, err := ioutil.ReadDir (fn)
	if err != nil {
		return fmt.Errorf("Cannot open %v: %v", fn, err)
	}

	for _, bfn := range entries {
		if bfn.IsDir() ||
			bfn.Name() == "sitemap.xml.gz" {
			continue
		}

		n := strings.TrimSuffix(bfn.Name(), ".gz")

		if filepath.Ext(n) == ".html" && !bfn.ModTime().IsZero() {
			sitemapEntries[dir + "/" + n] = bfn.ModTime()
		}
	}
	return nil
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

		// openSUSE Tumbleweed has ~11000 package entries, 120000 should
		// be good enough as start
		sitemapEntries := make(map[string]time.Time, 120000)

		fn := filepath.Join(*servingDir, sfi.Name())
		entrydirs, err := ioutil.ReadDir (fn)
		if err != nil {
			return fmt.Errorf("Cannot open %v: %v", fn, err)
		}

		for _, bfn := range entrydirs {
			if bfn.Name() == "sitemap.xml.gz" {
				continue
			}

			if !bfn.ModTime().IsZero() {
				if bfn.IsDir() {
					collectFiles(fn, bfn.Name(), sitemapEntries)
				} else {
					sitemapEntries[bfn.Name()] = bfn.ModTime()
				}
			}

		}


		escapedUrlPath := &url.URL{Path: sfi.Name()}
		if *verbose {
			log.Printf("Writing %d entries to %s/%s", len(sitemapEntries), dir, escapedUrlPath)
		}

		// Split sitemapEntries in smaller chunks
		// Google has a limit of 50.000 entries per file
		count := 0
		chunkSize := 45000
		batchKeys := make([]string, 0, chunkSize)
		saveChunks := func() error {
			chunk := make(map[string]time.Time, len(batchKeys))
			for _, v := range batchKeys {
				chunk[v] = sitemapEntries[v]
			}
			batchKeys = batchKeys[:0]

			sitemapPath := filepath.Join(dir, sfi.Name(), "sitemap" + strconv.Itoa(count) + ".xml.gz")
			if *verbose {
				log.Printf("Writing %d entries to %s", len(chunk), sitemapPath)
			}
			if err := write.Atomically(sitemapPath, true, func(w io.Writer) error {
				return sitemap.WriteTo(w, baseURL+"/" + escapedUrlPath.String(), chunk)
			}); err != nil {
				return fmt.Errorf("Write sitemap for %v failed: %v", sfi.Name(), err)
			}
			st, err := os.Stat(sitemapPath)
			if err == nil {
				sitemaps[escapedUrlPath.String() + "/sitemap" + strconv.Itoa(count) + ".xml"] = st.ModTime()
			}
			count++

			return nil
		}

		for k := range sitemapEntries {
			batchKeys = append(batchKeys, k)
			if len(batchKeys) == chunkSize {
				err = saveChunks()
				if err != nil {
					return err
				}
			}
		}
		// Process last, potentially incomplete batch
		if len(batchKeys) > 0 {
			err = saveChunks()
			if err != nil {
				return err
			}
		}
	}

	if *verbose {
		log.Printf("Writing %d entries to sitemapindex.xml", len(sitemaps))
	}
	return write.Atomically(filepath.Join(dir, "sitemapindex.xml.gz"), true, func(w io.Writer) error {
		return sitemap.WriteIndexTo(w, baseURL, sitemaps)
	})
}
