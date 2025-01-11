package wantdb

import (
	"context"

	"github.com/blobcache/glfs"
	"github.com/jmoiron/sqlx"
	"go.brendoncarroll.net/state/posixfs"
	"wantbuild.io/want/internal/migrations"
	"wantbuild.io/want/internal/wantdb/dbmig"

	_ "modernc.org/sqlite"
)

// Open opens a database in the file at p.
// It creates one if it does not exist
func Open(p string) (*sqlx.DB, error) {
	return sqlx.Open("sqlite", "file:"+p+"?cache=shared&mode=rwc")
}

// NewMemory creates an in memory database
func NewMemory() *sqlx.DB {
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		panic(err)
	}
	db.SetMaxOpenConns(1)
	return db
}

func Setup(ctx context.Context, db *sqlx.DB) error {
	s := migrations.InitialState()
	for _, q := range dbmig.ListMigrations() {
		s = s.ApplyStmt(q)
	}
	return migrations.Migrate(ctx, db, s)
}

func Import(tx sqlx.Tx, fsx posixfs.FS) (glfs.Ref, error) {
	return glfs.Ref{}, nil
}
