package main

import (
	"io"
	"sync/atomic"

	pb "github.com/thkukuk/rpm2docserv/pkg/proto"
	"github.com/thkukuk/rpm2docserv/pkg/write"
	"google.golang.org/protobuf/proto"
)

// writeIndex serializes an index for the redirect package (used in
// docserv-auxserver) to dest.
func writeIndex(dest string, gv globalView) error {
	idx := &pb.Index{
		Entry: make([]*pb.IndexEntry, 0, len(gv.xref)),
	}

	langs := make(map[string]bool)
	sections := make(map[string]bool)
	for _, x := range gv.xref {
		for _, m := range x {
			idx.Entry = append(idx.Entry, &pb.IndexEntry{
				Name:      m.Name,
				Suite:     m.Package.Suite,
				Binarypkg: m.Package.Binarypkg,
				Section:   m.Section,
				Language:  m.Language,
			})
			langs[m.Language] = true
			sections[m.Section] = true
			sections[m.MainSection()] = true
		}
	}

	for lang := range langs {
		idx.Language = append(idx.Language, lang)
	}

	for section := range sections {
		idx.Section = append(idx.Section, section)
	}

	idx.Suite = gv.idxSuites

	idxb, err := proto.Marshal(idx)
	if err != nil {
		return err
	}

	return write.Atomically(dest, false, func(w io.Writer) error {
		_, err := w.Write(idxb)
		if err != nil {
			return err
		}
		atomic.AddUint64(&gv.stats.IndexBytes, uint64(len(idxb)))
		return nil
	})
}
