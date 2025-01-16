package wantcmd

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/star"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/lib/want"
)

var importCmd = star.Command{
	Metadata: star.Metadata{Short: "import the repository, then exit"},
	Flags:    []star.IParam{dbParam},
	F: func(c star.Context) error {
		ctx := c.Context
		startTime := time.Now()
		repo, err := openRepo()
		if err != nil {
			return err
		}
		db := dbParam.Load(c)
		srcid, err := want.Import(ctx, db, repo)
		if err != nil {
			return err
		}
		dur := time.Since(startTime)
		root, s, err := want.AccessSource(ctx, db, srcid)
		if err != nil {
			return err
		}
		if err := printTreeRef(ctx, s, c.StdOut, *root); err != nil {
			return err
		}
		fmt.Fprintf(c.StdErr, "%v\n", dur)
		return c.StdOut.Flush()
	},
}

func printTreeRef(ctx context.Context, store cadata.Getter, w io.Writer, treeRef glfs.Ref) error {
	ag := glfs.NewAgent()
	return ag.WalkTree(ctx, store, treeRef, func(prefix string, ent glfs.TreeEntry) error {
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
