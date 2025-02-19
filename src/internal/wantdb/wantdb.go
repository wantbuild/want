package wantdb

import (
	"context"
	"errors"

	"github.com/jmoiron/sqlx"
	"wantbuild.io/want/src/internal/migrations"
	"wantbuild.io/want/src/internal/wantdb/dbmig"

	_ "modernc.org/sqlite"
)

// Open opens a database in the file at p.
// It creates one if it does not exist
func Open(p string) (*sqlx.DB, error) {
	if p == "" {
		return nil, errors.New("wantdb.Open: empty path")
	}
	// How To for PRAGMAs with the modernc.org/sqlite driver
	// https://pkg.go.dev/modernc.org/sqlite@v1.34.4#Driver.Open
	db, err := sqlx.Open("sqlite", "file:"+p+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // TODO: remove
	return db, nil
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
	if err := migrations.Migrate(ctx, db, s); err != nil {
		return err
	}
	return nil
}
