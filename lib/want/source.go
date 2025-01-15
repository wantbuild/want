package want

import (
	"context"
	"errors"

	"github.com/blobcache/glfs"
	"github.com/jmoiron/sqlx"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/dbutil"
	"wantbuild.io/want/internal/glfsport"
	"wantbuild.io/want/internal/wantdb"
	"wantbuild.io/want/lib/wantrepo"
)

type (
	SourceID = wantdb.SourceID
	Source   = wantdb.Source
)

// Import imports the repo into the database
func Import(ctx context.Context, db *sqlx.DB, repo *wantrepo.Repo) (wantdb.SourceID, error) {
	if repo == nil {
		return 0, errors.New("import requires a repo, got nil")
	}
	return dbutil.DoTx1(ctx, db, func(tx *sqlx.Tx) (SourceID, error) {
		sid, err := wantdb.CreateStore(tx)
		if err != nil {
			return 0, err
		}
		dst := wantdb.NewTxStore(tx, sid)
		imp := glfsport.Importer{
			Store:  dst,
			Dir:    repo.RootPath(),
			Filter: repo.PathFilter,
			Cache:  &glfsport.MemCache{}, // TODO:
		}
		root, err := imp.Import(ctx, "")
		if err != nil {
			return 0, err
		}
		repoDir := repo.RootPath()
		// TODO: cleanup old source here?
		srcid, err := wantdb.CreateSource(tx, sid, *root, repoDir)
		if err != nil {
			return 0, err
		}
		return srcid, nil
	})
}

// AccessSource calls fn with the root of the source and a store containing
// all of the sources blobs.
func AccessSource(ctx context.Context, db *sqlx.DB, id SourceID, fn func(ctx context.Context, s cadata.Getter, root glfs.Ref) error) error {
	return dbutil.ROTx(ctx, db, func(tx *sqlx.Tx) error {
		ctx, cf := context.WithCancel(ctx)
		defer cf()
		src, err := wantdb.GetSource(tx, id)
		if err != nil {
			return err
		}
		s := wantdb.NewTxStore(tx, src.Store)
		return fn(ctx, s, src.Root)
	})
}
