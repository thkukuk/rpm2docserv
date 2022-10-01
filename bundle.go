package bundle

//go:generate sh -c "go run goembed.go -package bundled -var assets assets/header.tmpl assets/footer.tmpl assets/style.css assets/chameleon.css assets/manpage.tmpl assets/manpageerror.tmpl assets/manpagefooterextra.tmpl assets/contents.tmpl assets/pkgindex.tmpl assets/srcpkgindex.tmpl assets/index.tmpl assets/about.tmpl assets/notfound.tmpl assets/favicon.ico assets/icon.svg assets/fallback-icon.svg > pkg/bundled/GENERATED_bundled.go"
