package wantdb

import (
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"wantbuild.io/want/internal/testutil"
)

func TestSetup(t *testing.T) {
	ctx := testutil.Context(t)
	tmpDB, err := Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	for _, db := range []*sqlx.DB{
		NewMemory(),
		tmpDB,
	} {
		require.NoError(t, Setup(ctx, db))
	}
}
