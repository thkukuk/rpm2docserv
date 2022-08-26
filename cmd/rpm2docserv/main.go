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

        "github.com/thkukuk/rpm2docserv/pkg/bundled"
        "github.com/thkukuk/rpm2docserv/pkg/commontmpl"
	"github.com/thkukuk/rpm2docserv/pkg/write"
)

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

	forceRerender = flag.Bool("force_rerender",
		true,
		"Forces all manpages to be re-rendered, even if they are up to date")


	injectAssets = flag.String("inject-assets",
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
)

// use go build -ldflags "-X main.rpm2docservVersion=<version>" to set the version
var rpm2docservVersion = "HEAD"

func logic() error {
	start := time.Now()

	// Stage 1: Download specified packages and their dependencies
	if !*noDownload {
		log.Printf("Downloading RPMs...\n");
		err := zypperDownload(strings.Split(*pkg2Render, ","), *cacheDir, start)
		if err != nil {
			return fmt.Errorf("downloading packages: %v", err)
		}
	}
	stage2 := time.Now()

	/* Stage 2: build globalView.pkgs by reading from disk */
	log.Printf("Gathering all packages...\n");
	globalView, err := buildGlobalView (*cacheDir, start)
	log.Printf("Gathered all packages, total %d packages", len(globalView.pkgs))

	stage3 := time.Now()

	// Stage 3: Extract manual pages from packages and rename them
	err = extractManpagesAll(*cacheDir, *servingDir, globalView)
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

func main() {
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if *showVersion || *verbose {
		fmt.Printf("rpm2docserv %s\n", rpm2docservVersion)
		if !*verbose {
			return
		}
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

	// All of our .so references are relative to *servingDir. For
	// mandoc(1) to find the files, we need to change the working
	// directory now. But first make sure it exists.
	if err := os.MkdirAll(*servingDir, os.ModePerm); err != nil {
		log.Fatal(err)
	}

	if err := os.Chdir(*servingDir); err != nil {
		log.Fatal(err)
	}

	if err := logic(); err != nil {
		log.Fatal(err)
	}
}
