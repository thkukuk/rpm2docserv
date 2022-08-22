package main

import (
	"flag"
	"fmt"
	//"io"
	"log"
	"os"
	//"path/filepath"
	"strings"
	"time"

	_ "net/http/pprof"

	//"github.com/Debian/debiman/internal/bundled"
	//"github.com/Debian/debiman/internal/commontmpl"
	//"github.com/Debian/debiman/internal/write"
)

var (
	servingDir = flag.String("serving_dir",
		"/srv/docserv",
		"Directory in which to place the manpages which should be served")

	indexPath = flag.String("index",
		"<serving_dir>/auxserver.idx",
		"Path to an auxserver index to generate")

	pkg2Render = flag.String("packages",
		"patterns-microos-defaults,patterns-containers-container_runtime,patterns-microos-selinux",
		"Comma separated list of packages to extract documentation from")

	forceRerender = flag.Bool("force_rerender",
		false,
		"Forces all manpages to be re-rendered, even if they are up to date")

	forceReextract = flag.Bool("force_reextract",
		false,
		"Forces all manpages to be re-extracted, even if there is no newer package version")

	remoteMirror = flag.String("remote_mirror",
		"http://localhost:3142/deb.debian.org/",
		"URL of a Debian mirror to fetch packages from. localhost:3142 is provided by apt-cacher-ng")

	localMirror = flag.String("local_mirror",
		"",
		"If non-empty, a file system path to a Debian mirror, e.g. /srv/mirrors/debian on DSA-maintained machines")

	injectAssets = flag.String("inject_assets",
		"",
		"If non-empty, a file system path to a directory containing assets to overwrite")

	alternativesDir = flag.String("alternatives_dir",
		"",
		"If non-empty, a directory containing JSON-encoded lists of slave alternative links, named after the suite (e.g. sid.json.gz, testing.json.gz, etc.)")

	showVersion = flag.Bool("version",
		false,
		"Show rpm2docserv version and exit")
)

// use go build -ldflags "-X main.rpm2docservVersion=<version>" to set the version
var rpm2docservVersion = "HEAD"

// TODO: handle deleted packages, i.e. packages which are present on
// disk but not in pkgs

func logic() error {
	start := time.Now()

	// Stage 1: Download specified packages and their dependencies
	err := zypperDownload(strings.Split(*pkg2Render, ","), start)
	if err != nil {
		return fmt.Errorf("downloading packages: %v", err)
	}

	// Stage 1: all Debian packages of all architectures of the
	// specified suites are discovered.
//	globalView, err := buildGlobalView(ar, distributions(
//		strings.Split(*syncCodenames, ","),
//		strings.Split(*syncSuites, ",")),
//		*alternativesDir,
//		start)
//	if err != nil {
//		return fmt.Errorf("gathering packages: %v", err)
//	}

//	log.Printf("gathered packages of all suites, total %d packages", len(globalView.pkgs))

	// Stage 2: man pages and auxiliary files (e.g. content fragment
	// files which are included by a number of manpages) are extracted
	// from the identified Debian packages.
//	if err := parallelDownload(ar, globalView); err != nil {
//		return fmt.Errorf("extracting manpages: %v", err)
//	}

//	log.Printf("Extracted all manpages, now rendering")

	// Stage 3: all man pages are rendered into an HTML representation
	// using mandoc(1), directory index files are rendered, contents
	// files are rendered.
//	if err := renderAll(globalView); err != nil {
//		return fmt.Errorf("rendering manpages: %v", err)
//	}

//	log.Printf("Rendered all manpages, writing index")

	// Stage 4: write the index only after all rendering is complete,
	// otherwise debiman-auxserver might serve redirects to pages
	// which cannot be served yet.
//	path := strings.Replace(*indexPath, "<serving_dir>", *servingDir, -1)
//	log.Printf("Writing debiman-auxserver index to %q", path)
//	if err := writeIndex(path, globalView); err != nil {
//		return fmt.Errorf("writing index: %v", err)
//	}

//	if err := renderAux(*servingDir, globalView); err != nil {
//		return fmt.Errorf("rendering aux files: %v", err)
//	}

//	fmt.Printf("total number of packages: %d\n", len(globalView.pkgs))
//	fmt.Printf("packages extracted:       %d\n", globalView.stats.PackagesExtracted)
//	fmt.Printf("packages deleted:         %d\n", globalView.stats.PackagesDeleted)
//	fmt.Printf("manpages rendered:        %d\n", globalView.stats.ManpagesRendered)
//	fmt.Printf("total manpage bytes:      %d\n", globalView.stats.ManpageBytes)
//	fmt.Printf("total HTML bytes:         %d\n", globalView.stats.HTMLBytes)
//	fmt.Printf("auxserver index bytes:    %d\n", globalView.stats.IndexBytes)
//	fmt.Printf("wall-clock runtime (s):   %d\n", int(time.Now().Sub(start).Seconds()))

//	return write.Atomically(filepath.Join(*servingDir, "metrics.txt"), false, func(w io.Writer) error {
//		if err := writeMetrics(w, globalView, start); err != nil {
//			return fmt.Errorf("writing metrics: %v", err)
//		}
//		return nil
	//	})
	return nil
}

func main() {
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if *showVersion {
		fmt.Printf("rpm2docserv %s\n", rpm2docservVersion)
		return
	}

//	if *injectAssets != "" {
//		if err := bundled.Inject(*injectAssets); err != nil {
//			log.Fatal(err)
//		}

//		commonTmpls = commontmpl.MustParseCommonTmpls()
//		contentsTmpl = mustParseContentsTmpl()
//		pkgindexTmpl = mustParsePkgindexTmpl()
//		srcpkgindexTmpl = mustParseSrcPkgindexTmpl()
//		indexTmpl = mustParseIndexTmpl()
//		faqTmpl = mustParseFaqTmpl()
//		aboutTmpl = mustParseAboutTmpl()
//		manpageTmpl = mustParseManpageTmpl()
//		manpageerrorTmpl = mustParseManpageerrorTmpl()
//		manpagefooterextraTmpl = mustParseManpagefooterextraTmpl()
//	}

	// All of our .so references are relative to *servingDir. For
	// mandoc(1) to find the files, we need to change the working
	// directory now.
	if err := os.Chdir(*servingDir); err != nil {
		log.Fatal(err)
	}

	if err := logic(); err != nil {
		log.Fatal(err)
	}
}
