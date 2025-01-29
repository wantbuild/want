package wantops

import (
	"testing"

	"github.com/blobcache/glfs"
	"github.com/stretchr/testify/require"

	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/internal/testutil"
)

func TestPostGetBuildTask(t *testing.T) {
	ctx := testutil.Context(t)
	s := stores.NewMem()

	main, err := glfs.PostTree(ctx, s, glfs.Tree{})
	require.NoError(t, err)
	x := BuildTask{
		Main: *main,
		Metadata: map[string]any{
			"a": "a",
			"b": "b",
			"c": "c",
		},
	}
	ref, err := PostBuildTask(ctx, s, x)
	require.NoError(t, err)
	y, err := GetBuildTask(ctx, s, *ref)
	require.NoError(t, err)
	require.Equal(t, x, *y)
}

func TestPostGetCompileTask(t *testing.T) {
	ctx := testutil.Context(t)
	s := stores.NewMem()

	modRef, err := glfs.PostTree(ctx, s, glfs.Tree{})
	require.NoError(t, err)
	x := CompileTask{
		Module:   *modRef,
		Metadata: map[string]any{"abc": "123"},
	}
	ref, err := PostCompileTask(ctx, s, x)
	require.NoError(t, err)
	y, err := GetCompileTask(ctx, s, *ref)
	require.NoError(t, err)

	require.Equal(t, x, *y)
}
