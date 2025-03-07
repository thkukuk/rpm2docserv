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

type Products struct {
	Name     string   `yaml:"name"`
	Cache    []string `yaml:"cache,omitempty"`
	Packages []string `yaml:"packages,omitempty"`
	Alias    []string `yaml:"alias,omitempty"`
}

type Config struct {
        ProjectName string     `yaml:"projectname,omitempty"`
        ProjectUrl  string     `yaml:"projecturl,omitempty"`
        LogoUrl     string     `yaml:"logourl,omitempty"`
        AssetsDir   string     `yaml:"assets,omitempty"`
        ServingDir  string     `yaml:"servingdir"`
        IndexPath   string     `yaml:"auxindex"`
        Download    string     `yaml:"download"`
        IsOffline   bool       `yaml:"offline,omitempty"`
        BaseUrl     string     `yaml:"baseurl,omitempty"`
        Products    []Products `yaml:"products"`
        SortOrder   []string   `yaml:"sortorder"`
}

func read_yaml_config(conffile string) (Config, error) {

        var config Config

        file, err := os.ReadFile(conffile)
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

	if (*baseURL)[len(*baseURL)-1:] == "/" {
		t := (*baseURL)[:len(*baseURL)-1]
		baseURL = &t
	}

	log.Printf("docserv sitemap generation for %q", *servingDir)

	err := walkDirs(*servingDir, *baseURL)
	if err != nil {
		log.Fatal(err)
	}
}

func collectFiles(basedir string, dir string, sitemapEntries map[string]time.Time) error {

	var fn string
	var fp string // prefix with directory and "/" if dir is not empty

	if len(dir) > 0 {
		fn = filepath.Join(basedir, dir)
		fp = dir + "/"
	} else {
		fn = basedir
		fp = ""
	}
	entries, err := ioutil.ReadDir (fn)
	if err != nil {
		return fmt.Errorf("Cannot open %v: %v", fn, err)
	}

	for _, bfn := range entries {
		if bfn.IsDir() ||
			(strings.HasPrefix(bfn.Name(), "sitemap") &&
			strings.HasSuffix(bfn.Name(), ".xml.gz")) {
			continue
		}

		n := strings.TrimSuffix(bfn.Name(), ".gz")

		if filepath.Ext(n) == ".html" && !bfn.ModTime().IsZero() {
			// For index.html only add the directory URL
			if n == "index.html" {
				sitemapEntries[fp] = bfn.ModTime()
			} else {
				sitemapEntries[fp + n] = bfn.ModTime()
			}
		}
	}
	return nil
}

func writeSitemap(basedir string, product string, baseUrl string,
	          sitemapEntries map[string]time.Time, sitemaps map[string]time.Time) error {

	escapedUrlPath := &url.URL{Path: product}
	if *verbose {
		log.Printf("Found %d entries for %s/%s", len(sitemapEntries), basedir, escapedUrlPath)
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

		sitemapPath := filepath.Join(basedir, product, "sitemap" + strconv.Itoa(count) + ".xml.gz")
		if *verbose {
			log.Printf("Writing %d entries to %s", len(chunk), sitemapPath)
		}

		urlPrefix := baseUrl
		if len(escapedUrlPath.String()) > 0 {
			urlPrefix = urlPrefix + "/" + escapedUrlPath.String()
		}
		if err := write.Atomically(sitemapPath, true, func(w io.Writer) error {
			return sitemap.WriteTo(w, urlPrefix, chunk)
		}); err != nil {
			return fmt.Errorf("Write sitemap for %v failed: %v", product, err)
		}
		st, err := os.Stat(sitemapPath)
		if err == nil {
			fn := filepath.Join(escapedUrlPath.String(), "sitemap" + strconv.Itoa(count) + ".xml")
			sitemaps[fn] = st.ModTime()
		}
		count++

		return nil
	}

	for k := range sitemapEntries {
		batchKeys = append(batchKeys, k)
		if len(batchKeys) == chunkSize {
			err := saveChunks()
			if err != nil {
				return err
			}
		}
	}
	// Process last, potentially incomplete batch
	if len(batchKeys) > 0 {
		err := saveChunks()
		if err != nil {
			return err
		}
	}

	return nil
}

func walkDirs(dir string, baseURL string) error {
	sitemaps := make(map[string]time.Time)

	/* Collect files in root directory */
	sitemapRootEntries := make(map[string]time.Time, 10)
	collectFiles(dir, "", sitemapRootEntries)
	err := writeSitemap(dir, "", baseURL, sitemapRootEntries, sitemaps)
	if err != nil {
		return err
	}

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

		// openSUSE Tumbleweed has ~11000 package entries, 140000 should
		// be good enough as start
		sitemapEntries := make(map[string]time.Time, 140000)

		fn := filepath.Join(*servingDir, sfi.Name())
		entrydirs, err := ioutil.ReadDir (fn)
		if err != nil {
			return fmt.Errorf("Cannot open %v: %v", fn, err)
		}

		for _, bfn := range entrydirs {
			// Ignore all sitemap*.xml.gz files
			if strings.HasPrefix(bfn.Name(), "sitemap") &&
				strings.HasSuffix(bfn.Name(), ".xml.gz") {
				continue
			}

			if !bfn.ModTime().IsZero() {
				if bfn.IsDir() {
					collectFiles(fn, bfn.Name(), sitemapEntries)
				} else {
					if bfn.Name() == "index.html" {
						sitemapEntries[""] = bfn.ModTime()
					} else {
						sitemapEntries[bfn.Name()] = bfn.ModTime()
					}
				}
			}

		}

		err = writeSitemap(dir, sfi.Name(), baseURL, sitemapEntries, sitemaps)
		if err != nil {
			return err
		}

	}

	if *verbose {
		log.Printf("Writing %d entries to sitemapindex.xml", len(sitemaps))
	}
	return write.Atomically(filepath.Join(dir, "sitemapindex.xml.gz"), true, func(w io.Writer) error {
		return sitemap.WriteIndexTo(w, baseURL, sitemaps)
	})
}
