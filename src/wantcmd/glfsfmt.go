package wantcmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"blobcache.io/glfs"
	"go.brendoncarroll.net/state/cadata"
	"go.brendoncarroll.net/stdctx/units"
)

func fmtTree(w io.Writer, tree []glfs.TreeEntry) error {
	var longest int
	for _, ent := range tree {
		if len(ent.Name) > longest {
			longest = len(ent.Name)
		}
	}
	format := "%-12s %-4s %-8s %-16s %s\n"
	if _, err := fmt.Fprintf(w, format, "MODE", "TYPE", "SIZE", "CONTENT_ID", "NAME"); err != nil {
		return err
	}
	for _, ent := range tree {
		_, err := fmt.Fprintf(w, format, ent.FileMode, fmtType(ent.Ref), fmtRefSize(ent.Ref), fmtCID(ent.Ref, true), ent.Name)
		if err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

func fmtType(x glfs.Ref) string {
	return string(x.Type)
}

func fmtRefSize(x glfs.Ref) string {
	switch x.Type {
	case glfs.TypeTree:
		minSize := base64.RawURLEncoding.EncodedLen(64)
		maxLen := x.Size / uint64(minSize)
		if maxLen == 0 {
			return "0"
		}
		return fmt.Sprintf("<=%d", maxLen)
	default:
		return units.FmtFloat64(float64(x.Size), "B")
	}
}

func printTreeRec(ctx context.Context, store cadata.Getter, w io.Writer, treeRef glfs.Ref) error {
	if _, err := store.Get(ctx, treeRef.CID, make([]byte, store.MaxSize())); err != nil {
		return err
	}
	return glfs.WalkTree(ctx, store, treeRef, func(prefix string, ent glfs.TreeEntry) error {
		depth := 0
		if prefix != "" {
			depth = strings.Count(prefix, "/") + 1
		}
		indent := ""
		for i := 0; i < depth; i++ {
			indent += " "
		}
		name := ent.Name
		if ent.Ref.Type == glfs.TypeTree {
			name += "/"
		}
		_, err := fmt.Fprintf(w, "%-70s %s\n", indent+name, fmtCID(ent.Ref, true))
		return err
	})
}

func fmtCID(x glfs.Ref, short bool) string {
	ret := x.CID.String()
	if short {
		ret = ret[:16]
	}
	return ret
}
