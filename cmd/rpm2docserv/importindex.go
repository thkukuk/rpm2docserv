package main

import (
	"log"
	"strings"

	"github.com/thkukuk/rpm2docserv/pkg/redirect"
	"github.com/thkukuk/rpm2docserv/pkg/tag"
	"github.com/thkukuk/rpm2docserv/pkg/manpage"
)

func importIndex(index string, gv *globalView) error {

	idx, err := redirect.IndexFromProto(strings.Split(index,"#"))
        if err != nil {
		return err
        }

        log.Printf("Loaded %d manpage entries, %d products, %d languages, %d sections from index %q",
                len(idx.Entries), len(idx.ProductNames), len(idx.Langs), len(idx.Sections), index)

	for _, entries := range idx.Entries {
		for _, entry := range entries {
			pkg := &manpage.PkgMeta{
				Product: entry.Product,
				Binarypkg: entry.Binarypkg,
			}
			m := &manpage.Meta{
				Name: entry.Name,
				Section: entry.Section,
				Language: entry.Language,
				Package: pkg,
			}
			m.LanguageTag, _ = tag.FromLocale(m.Language)
			gv.xref[m.Name] = append(gv.xref[m.Name], m)
		}
	}

	return nil
}
