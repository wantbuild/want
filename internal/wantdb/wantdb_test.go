package wantdb

import (
	"testing"

	"github.com/stretchr/testify/require"
	"wantbuild.io/want/internal/testutil"
)

func TestSetup(t *testing.T) {
	ctx := testutil.Context(t)
	db := NewMemory()
	require.NoError(t, Setup(ctx, db))
}
