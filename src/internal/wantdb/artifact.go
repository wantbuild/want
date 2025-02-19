package wantdb

import (
	"github.com/jmoiron/sqlx"
	"go.brendoncarroll.net/state/cadata"
	"go.brendoncarroll.net/tai64"
	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/wantjob"
)

type ArtifactID = cadata.ID

func CreateArtifact(tx *sqlx.Tx, sch wantjob.Schema, fn func(dst cadata.Store) ([]byte, error)) (*ArtifactID, error) {
	sid, err := CreateStore(tx)
	if err != nil {
		return nil, err
	}
	root, err := fn(NewTxStore(tx, sid))
	if err != nil {
		return nil, err
	}
	afid := stores.Hash(root)
	now := tai64.Now()
	if _, err := tx.Exec(`INSERT OR IGNORE INTO artifacts (id, store_id, root, sch, created_at)
		VALUES (?, ?, ?, ?, ?)`, afid, sid, root, sch, now.Marshal()); err != nil {
		return nil, err
	}
	return &afid, nil
}

// Artifacts are immutable datastructures consisting of
// some root data, and a set of transitively referenced content-addressable blobs.
type Artifact struct {
	Root      []byte
	Schema    wantjob.Schema
	Store     StoreID
	CreatedAt tai64.TAI64N
}

func GetArtifact(tx *sqlx.Tx, id ArtifactID) (*Artifact, error) {
	var row struct {
		Root      []byte  `db:"root"`
		Schema    string  `db:"sch"`
		Store     StoreID `db:"store_id"`
		CreatedAt []byte  `db:"created_at"`
	}
	if err := tx.Get(&row, `SELECT store_id, root, sch, created_at FROM artifacts WHERE id = ?`, id); err != nil {
		return nil, err
	}
	return &Artifact{
		Root:   row.Root,
		Store:  row.Store,
		Schema: wantjob.Schema(row.Schema),
	}, nil
}
