package wantops

import (
	"testing"

	"github.com/blobcache/glfs"
	"github.com/stretchr/testify/require"

	"wantbuild.io/want/internal/stores"
	"wantbuild.io/want/internal/testutil"
)

func TestPostGetCompileTask(t *testing.T) {
	ctx := testutil.Context(t)
	s := stores.NewMem()

	ground, err := glfs.PostTree(ctx, s, glfs.Tree{})
	require.NoError(t, err)
	x := CompileTask{
		Ground:   *ground,
		Metadata: map[string]any{"abc": "123"},
	}
	ref, err := PostCompileTask(ctx, s, x)
	require.NoError(t, err)
	y, err := GetCompileTask(ctx, s, *ref)
	require.NoError(t, err)

	require.Equal(t, x, *y)
}
