package wantcmd

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/blobcache/glfs"
	"github.com/jmoiron/sqlx"
	"go.brendoncarroll.net/star"
	"go.brendoncarroll.net/state/cadata"
	"wantbuild.io/want/internal/dbutil"
	"wantbuild.io/want/internal/glfsport"
	"wantbuild.io/want/internal/wantdb"
	"wantbuild.io/want/lib/wantrepo"
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
		sid, root, err := importRepo(c.Context, db, repo)
		if err != nil {
			return err
		}
		if err := dbutil.ROTx(ctx, db, func(tx *sqlx.Tx) error {
			s := wantdb.NewStore(tx, sid)
			return printTreeRef(ctx, s, c.StdOut, *root)
		}); err != nil {
			return err
		}
		dur := time.Since(startTime)
		fmt.Fprintf(c.StdErr, "%v\n", dur)
		return c.StdOut.Flush()
	},
}

func importRepo(ctx context.Context, db *sqlx.DB, repo *wantrepo.Repo) (wantdb.StoreID, *glfs.Ref, error) {
	return dbutil.DoTx2(ctx, db, func(tx *sqlx.Tx) (wantdb.StoreID, *glfs.Ref, error) {
		sid, err := wantdb.CreateStore(tx)
		if err != nil {
			return 0, nil, err
		}
		dst := wantdb.NewStore(tx, sid)
		imp := glfsport.Importer{
			Store:  dst,
			Dir:    repo.RootPath(),
			Filter: repo.PathFilter,
			Cache:  &glfsport.MemCache{}, // TODO:
		}
		root, err := imp.Import(ctx, "")
		if err != nil {
			return 0, nil, err
		}
		return sid, root, nil
	})
}

func glfsLs(ctx context.Context, db *sqlx.DB, root glfs.Ref, w io.Writer) error {
	return dbutil.DoTx(ctx, db, func(tx *sqlx.Tx) error {
		sid, err := dbutil.GetTx[wantdb.StoreID](tx, `SELECT id FROM stores LIMIT 1`)
		if err != nil {
			return err
		}
		s := wantdb.NewStore(tx, sid)
		return glfs.WalkTree(ctx, s, root, func(prefix string, tree glfs.TreeEntry) error {
			p := path.Join(prefix, tree.Name)
			_, err := fmt.Fprintf(w, "%s => %v %v\n", p, tree.FileMode, tree.Ref)
			return err
		})
	})
}

func printTreeRef(ctx context.Context, store cadata.Store, w io.Writer, treeRef glfs.Ref) error {
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
		fmt.Fprintf(w, "%-70s %s\n", indent+name, fmtCID(ent.Ref, true))
		return nil
	})
}

func fmtCID(x glfs.Ref, short bool) string {
	ret := x.CID.String()
	if short {
		ret = ret[:16]
	}
	return ret
}
