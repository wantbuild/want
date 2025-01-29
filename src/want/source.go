package want

import (
	"context"
	"errors"

	"github.com/blobcache/glfs"
	"github.com/jmoiron/sqlx"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/dbutil"
	"wantbuild.io/want/src/internal/glfsport"
	"wantbuild.io/want/src/internal/wantdb"
	"wantbuild.io/want/src/wantrepo"
)

type (
	SourceID = wantdb.SourceID
	Source   = wantdb.Source
)

// Import imports the repo into the database
func (sys *System) Import(ctx context.Context, repo *wantrepo.Repo) (wantdb.SourceID, error) {
	if repo == nil {
		return 0, errors.New("import requires a repo, got nil")
	}
	return dbutil.DoTx1(ctx, sys.db, func(tx *sqlx.Tx) (SourceID, error) {
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
func (sys *System) AccessSource(ctx context.Context, id SourceID) (*glfs.Ref, cadata.Getter, error) {
	src, err := dbutil.ROTx1(ctx, sys.db, func(tx *sqlx.Tx) (*wantdb.Source, error) {
		return wantdb.GetSource(tx, id)
	})
	if err != nil {
		return nil, nil, err
	}
	return &src.Root, wantdb.NewDBStore(sys.db, src.Store), nil
}
