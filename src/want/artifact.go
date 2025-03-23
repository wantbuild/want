package want

import (
	"context"
	"encoding/json"
	"errors"

	"blobcache.io/glfs"
	"github.com/jmoiron/sqlx"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/dbutil"
	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/internal/wantdb"
	"wantbuild.io/want/src/internal/wantrepo"
	"wantbuild.io/want/src/wantjob"
)

type (
	ArtifactID = wantdb.ArtifactID
)

// Import imports the repo into the database
func (sys *System) Import(ctx context.Context, repo *wantrepo.Repo) (*wantdb.ArtifactID, error) {
	if repo == nil {
		return nil, errors.New("import requires a repo, got nil")
	}
	return dbutil.DoTx1(ctx, sys.db, func(tx *sqlx.Tx) (*ArtifactID, error) {
		afid, err := wantdb.CreateArtifact(tx, wantjob.Schema_GLFS, func(dst cadata.Store) ([]byte, error) {
			root, err := repo.Import(ctx, dst, "")
			if err != nil {
				return nil, err
			}
			return json.Marshal(*root)
		})
		if err != nil {
			return nil, err
		}
		return afid, nil
	})
}

type Artifact struct {
	Root   []byte
	Schema wantjob.Schema
	Store  cadata.Getter
}

func (a Artifact) GLFS() (*glfs.Ref, error) {
	return glfstasks.ParseGLFSRef(a.Root)
}

// ViewArtifact returns an Artifact from the database.
func (sys *System) ViewArtifact(ctx context.Context, id ArtifactID) (*Artifact, error) {
	af, err := dbutil.ROTx1(ctx, sys.db, func(tx *sqlx.Tx) (*wantdb.Artifact, error) {
		return wantdb.GetArtifact(tx, id)
	})
	if err != nil {
		return nil, err
	}
	return &Artifact{
		Schema: af.Schema,
		Root:   af.Root,
		Store:  wantdb.NewDBStore(sys.db, af.Store),
	}, nil
}
