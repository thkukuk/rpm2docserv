package bundle

//go:generate sh -c "go run goembed.go -package bundled -var assets assets/header.tmpl assets/footer.tmpl assets/style.css assets/manpage.tmpl assets/manpageerror.tmpl assets/manpagefooterextra.tmpl assets/contents.tmpl assets/pkgindex.tmpl assets/srcpkgindex.tmpl assets/index.tmpl assets/about.tmpl assets/notfound.tmpl assets/opensearch.xml > pkg/bundled/GENERATED_bundled.go"
