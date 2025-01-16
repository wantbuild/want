package wantdb

import (
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"wantbuild.io/want/internal/dbutil"
	"wantbuild.io/want/internal/testutil"
	"wantbuild.io/want/internal/wantjob"
)

func TestSetup(t *testing.T) {
	ctx := testutil.Context(t)

	for _, mkdb := range []func() *sqlx.DB{
		NewMemory,
		func() *sqlx.DB {
			tmpDB, err := Open(filepath.Join(t.TempDir(), "test.db"))
			require.NoError(t, err)
			return tmpDB
		},
	} {
		require.NoError(t, Setup(ctx, mkdb()))
	}
}

func TestCreateJobs(t *testing.T) {
	ctx := testutil.Context(t)
	db := NewMemory()
	require.NoError(t, Setup(ctx, db))

	require.NoError(t, dbutil.DoTx(ctx, db, func(tx *sqlx.Tx) error {
		task := wantjob.Task{Op: "noop"}
		idx, err := CreateRootJob(tx, task)
		require.NoError(t, err)
		id := wantjob.JobID{idx}
		for i := 0; i < 10; i++ {
			idx, err := CreateChildJob(tx, id, task)
			require.NoError(t, err)
			id = append(id, idx)
		}
		return nil
	}))
}
