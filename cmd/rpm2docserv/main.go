package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	_ "net/http/pprof"

	"gopkg.in/yaml.v3"

        "github.com/thkukuk/rpm2docserv/pkg/bundled"
        "github.com/thkukuk/rpm2docserv/pkg/commontmpl"
	"github.com/thkukuk/rpm2docserv/pkg/write"
)

type Suites struct {
	Name     string   `yaml:"name"`
	Cache    []string `yaml:"cache,omitempty"`
	Packages []string `yaml:"packages,omitempty"`
	Alias    []string `yaml:"alias,omitempty"`
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

var (
	servingDir = flag.String("serving-dir",
		"/srv/docserv",
		"Directory in which to place the manpages which should be served")

	indexPath = flag.String("index",
		"<serving_dir>/auxserver.idx",
		"Path to an auxserver index to generate")

	pkg2Render = flag.String("packages",
		"patterns-microos-defaults,patterns-containers-container_runtime,patterns-microos-selinux,patterns-base-documentation",
		"Comma separated list of packages to extract documentation from")

	cacheDir = flag.String("cache",
		"/var/cache/rpm2docserv",
		"Directory in which the downloaded RPMs will be temporary stored")

	yamlConfig = flag.String("config",
		"",
		"Configuration file in yaml format")

	injectAssets = flag.String("assets",
		"",
		"If non-empty, a file system path to a directory containing assets to overwrite")

	noDownload = flag.Bool("no-download",
		false,
		"Use packages from local cache, no new download")

        verbose = flag.Bool("verbose",
		false,
		"Print additional status messages")

	showVersion = flag.Bool("version",
		false,
		"Show rpm2docserv version and exit")

	isOffline = false
	productName string
        productUrl  string
	logoUrl     string
	suites      []Suites
)

// use go build -ldflags "-X main.rpm2docservVersion=<version>" to set the version
var rpm2docservVersion = "HEAD"

func logic() error {
	start := time.Now()

	// Stage 1: Download specified packages and their dependencies
	// we don't do this if we have more than one cache directory.
	if !*noDownload {
		if len(suites) == 1 && len(suites[0].Cache) == 1 {
			log.Printf("Downloading RPMs...\n");
			err := zypperDownload(suites[0].Packages, suites[0].Cache[0], start)
			if err != nil {
				return fmt.Errorf("downloading packages: %v", err)
			}
		} else {
			log.Printf("Downloading RPMs... - skipped, more than one suite or cache directory specified")
		}
	}
	stage2 := time.Now()

	/* Stage 2: build globalView.pkgs by reading from disk */
	log.Printf("Gathering all packages...\n");
	globalView, err := buildGlobalView (suites, start)
	log.Printf("Gathered all packages, total %d packages", len(globalView.pkgs))

	stage3 := time.Now()

	// Stage 3: Extract manual pages from packages and rename them
	err = extractManpagesAll(*cacheDir, *servingDir, &globalView)
	if err != nil {
		return fmt.Errorf("extracing manual pages: %v", err)
	}
	log.Printf("Extracted all manpages")

	stage4 := time.Now()

	log.Printf("Rendering manpages...\n")
	// Stage 4: all man pages are rendered into an HTML representation
	// using mandoc(1), directory index files are rendered, contents
	// files are rendered.
	if err := renderAll(globalView); err != nil {
		return fmt.Errorf("rendering manpages: %v", err)
	}
	log.Printf("Rendered all manpages, writing index")

	stage5 := time.Now()

	// Stage 5: write the index after all rendering is complete.
	path := strings.Replace(*indexPath, "<serving_dir>", *servingDir, -1)
	log.Printf("Writing docserv-auxserver index to %q", path)
	if err := writeIndex(path, globalView); err != nil {
		return fmt.Errorf("writing index: %v", err)
	}

	if err := renderAux(*servingDir, globalView); err != nil {
		return fmt.Errorf("rendering aux files: %v", err)
	}

	finish := time.Now()

	fmt.Printf("total number of packages: %d\n", len(globalView.pkgs))
	fmt.Printf("packages with manpages:   %d\n", globalView.stats.PackagesExtracted)
	fmt.Printf("manpages rendered:        %d\n", globalView.stats.ManpagesRendered)
	fmt.Printf("total manpage bytes:      %d\n", globalView.stats.ManpageBytes)
	fmt.Printf("total HTML bytes:         %d\n", globalView.stats.HTMLBytes)
	fmt.Printf("auxserver index bytes:    %d\n", globalView.stats.IndexBytes)
	fmt.Printf("download packages (s):    %d\n", int(stage2.Sub(start).Seconds()))
	fmt.Printf("gather all packages (s):  %d\n", int(stage3.Sub(stage2).Seconds()))
	fmt.Printf("extract all manpages (s): %d\n", int(stage4.Sub(stage3).Seconds()))
	fmt.Printf("render all manpages (s):  %d\n", int(stage5.Sub(stage4).Seconds()))
	fmt.Printf("write index (s):          %d\n", int(finish.Sub(stage5).Seconds()))
	fmt.Printf("wall-clock runtime (s):   %d\n", int(finish.Sub(start).Seconds()))

	return write.Atomically(filepath.Join(*servingDir, "metrics.txt"), false, func(w io.Writer) error {
		if err := writeMetrics(w, globalView, start); err != nil {
			return fmt.Errorf("writing metrics: %v", err)
		}
		return nil
	})
	return nil
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

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	flag.Parse()

	if *showVersion || *verbose {
		fmt.Printf("rpm2docserv %s\n", rpm2docservVersion)
		if !*verbose {
			return
		}
	}

	if len(*yamlConfig) > 0 {
		config, err := read_yaml_config(*yamlConfig)
		if err != nil {
			log.Fatal(err)
		}
		if len(config.AssetsDir) > 0 {
			injectAssets = &config.AssetsDir
		}
		if len(config.ServingDir) > 0 {
			servingDir = &config.ServingDir
		}
		if len(config.IndexPath) > 0 {
			indexPath = &config.IndexPath
		}
		if len(config.Download) > 0 {
			if strings.EqualFold(config.Download, "false") {
				*noDownload = true
			} else if strings.EqualFold(config.Download, "true") {
				*noDownload = false
			} else {
				log.Fatal("Invalid value %q for option \"download\" in config %q",
					config.Download, yamlConfig)
			}
		}
		if len(config.SortOrder) > 0 {
			for idx, r := range config.SortOrder {
				sortOrder[r] = idx
			}
		}

		isOffline = config.IsOffline
		productName = config.ProductName
		productUrl = config.ProductUrl
		logoUrl = config.LogoUrl
		suites = config.Products
	} else {
		suites = make([]Suites, 1)
		suites[0].Name = "manpages"
		suites[0].Cache = append(suites[0].Cache, *cacheDir)
		suites[0].Packages = strings.Split(*pkg2Render, ",")
	}

	if *injectAssets != "" {
		if err := bundled.Inject(*injectAssets); err != nil {
			log.Fatal(err)
		}

		commonTmpls = commontmpl.MustParseCommonTmpls()
		contentsTmpl = mustParseContentsTmpl()
		pkgindexTmpl = mustParsePkgindexTmpl()
		srcpkgindexTmpl = mustParseSrcPkgindexTmpl()
		indexTmpl = mustParseIndexTmpl()
		aboutTmpl = mustParseAboutTmpl()
		manpageTmpl = mustParseManpageTmpl()
		manpageerrorTmpl = mustParseManpageerrorTmpl()
		manpagefooterextraTmpl = mustParseManpagefooterextraTmpl()
	}

	// make sure the serving directory exists
	if err := os.MkdirAll(*servingDir, os.ModePerm); err != nil {
		log.Fatal(err)
	}

	if err := logic(); err != nil {
		log.Fatal(err)
	}
}
