package glfsport

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"wantbuild.io/want/internal/stores"
	"wantbuild.io/want/internal/testutil"
)

// TestExportClean checks that exporting to an empty directory works.
func TestExportClean(t *testing.T) {
	ctx := testutil.Context(t)
	s := stores.NewMem()
	dir := t.TempDir()

	exp := Exporter{
		Cache: NullCache{},
		Store: s,
		Dir:   dir,
	}
	ref := testutil.PostFS(t, s, map[string][]byte{
		"a": []byte("aaaaaa"),
		"b": []byte("bbb"),
	})
	require.NoError(t, exp.Export(ctx, ref, ""))
}

func TestExportMultiple(t *testing.T) {
	ctx := testutil.Context(t)
	s := stores.NewMem()
	dir := t.TempDir()
	cache := &MemCache{}

	exp := Exporter{
		Cache: cache,
		Store: s,
		Dir:   dir,
	}
	ref1 := testutil.PostFS(t, s, map[string][]byte{
		"a": []byte("aaaaaa"),
		"b": []byte("bbb"),
	})
	require.NoError(t, exp.Export(ctx, ref1, ""))

	ref2 := testutil.PostFS(t, s, map[string][]byte{
		"a": []byte("aaa"),
		"b": []byte("bbb"),
	})
	require.NoError(t, exp.Export(ctx, ref2, ""))
}

func TestExportDirty(t *testing.T) {
	ctx := testutil.Context(t)
	s := stores.NewMem()
	dir := t.TempDir()
	cache := &MemCache{}

	exp := Exporter{
		Cache: cache,
		Store: s,
		Dir:   dir,
	}
	ref1 := testutil.PostFS(t, s, map[string][]byte{
		"a": []byte("aaaaaa"),
		"b": []byte("bbb"),
	})
	require.NoError(t, exp.Export(ctx, ref1, ""))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), []byte("AAAA"), 0o644))

	ref2 := testutil.PostFS(t, s, map[string][]byte{
		"a": []byte("aaa"),
		"b": []byte("bbb"),
	})

	err := exp.Export(ctx, ref2, "")
	require.Error(t, err)
	require.True(t, errors.As(err, &ErrStaleCache{}))
}
