package wantdb

import (
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"go.brendoncarroll.net/state/cadata/storetest"

	"wantbuild.io/want/src/internal/dbutil"
	"wantbuild.io/want/src/internal/testutil"
)

func TestStoreCreateDrop(t *testing.T) {
	ctx := testutil.Context(t)
	db := setup(t)
	require.NoError(t, dbutil.DoTx(ctx, db, func(tx *sqlx.Tx) error {
		sid, err := CreateStore(tx)
		require.NoError(t, err)
		t.Log(sid)
		require.NoError(t, DropStore(tx, sid))
		return nil
	}))
}

func TestTxStoreAPI(t *testing.T) {
	ctx := testutil.Context(t)
	db := setup(t)
	require.NoError(t, dbutil.DoTx(ctx, db, func(tx *sqlx.Tx) error {
		storetest.TestStore(t, func(t testing.TB) storetest.Store {
			if strings.Contains(t.Name(), "List") {
				t.SkipNow()
			}
			sid, err := CreateStore(tx)
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, DropStore(tx, sid))
			})
			return NewTxStore(tx, sid)
		})
		return nil
	}))
}

func TestDBStoreAPI(t *testing.T) {
	ctx := testutil.Context(t)
	db := setup(t)
	storetest.TestStore(t, func(t testing.TB) storetest.Store {
		if strings.Contains(t.Name(), "List") {
			t.SkipNow()
		}
		sid, err := dbutil.DoTx1(ctx, db, CreateStore)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, dbutil.DoTx(ctx, db, func(tx *sqlx.Tx) error { return DropStore(tx, sid) }))
		})
		return NewDBStore(db, sid)
	})
}

func setup(t testing.TB) *sqlx.DB {
	db := NewMemory()
	require.NoError(t, Setup(testutil.Context(t), db))
	return db
}
