package wantdb

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/blobcache/glfs"
	"github.com/jmoiron/sqlx"
	"go.brendoncarroll.net/tai64"
	"wantbuild.io/want/internal/migrations"
	"wantbuild.io/want/internal/wantdb/dbmig"

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
	return sqlx.Open("sqlite", "file:"+p+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
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

type SourceID = uint64

func CreateSource(tx *sqlx.Tx, sid StoreID, root glfs.Ref, repoPath string) (SourceID, error) {
	var ret SourceID
	rootData, err := json.Marshal(root)
	if err != nil {
		return 0, err
	}
	now := tai64.Now()
	if err := tx.Get(&ret, `INSERT INTO sources (store_id, root, repo_dir, created_at) VALUES (?, ?, ?, ?) RETURNING id`, sid, string(rootData), repoPath, now.Marshal()); err != nil {
		return 0, err
	}
	return ret, err
}

type Source struct {
	Root      glfs.Ref
	Store     StoreID
	RepoDir   string
	CreatedAt tai64.TAI64N
}

func GetSource(tx *sqlx.Tx, id SourceID) (*Source, error) {
	var row struct {
		Root      []byte  `db:"root"`
		Store     StoreID `db:"store_id"`
		RepoDir   string  `db:"repo_dir"`
		CreatedAt []byte  `db:"created_at"`
	}
	if err := tx.Get(&row, `SELECT store_id, root, repo_dir, created_at FROM sources WHERE id = ?`, id); err != nil {
		return nil, err
	}
	createdAt, err := tai64.ParseN(row.CreatedAt)
	if err != nil {
		return nil, err
	}
	var root glfs.Ref
	if err := json.Unmarshal(row.Root, &root); err != nil {
		return nil, err
	}
	return &Source{
		Root:      root,
		RepoDir:   row.RepoDir,
		Store:     row.Store,
		CreatedAt: createdAt,
	}, nil
}
