package testutil

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/blobcache/glfs"
	"github.com/blobcache/glfs/glfstar"
	"github.com/stretchr/testify/require"
	"go.brendoncarroll.net/state/cadata"
)

func PostFS(t testing.TB, s cadata.Store, m map[string][]byte) glfs.Ref {
	ctx := Context(t)
	ref, err := postFS(ctx, s, m)
	require.NoError(t, err)
	return *ref
}

func postFS(ctx context.Context, s cadata.Store, m map[string][]byte) (*glfs.Ref, error) {
	ag := glfs.NewAgent()
	var ents []glfs.TreeEntry
	for p, data := range m {
		ref, err := ag.PostBlob(ctx, s, bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		ents = append(ents, glfs.TreeEntry{
			Name:     p,
			FileMode: 0o700,
			Ref:      *ref,
		})
	}
	return glfs.PostTreeEntries(ctx, s, ents)
}

func PrintFS(t testing.TB, s cadata.Getter, x glfs.Ref) {
	t.Helper()
	ctx := Context(t)
	ag := glfs.NewAgent()
	if x.Type != glfs.TypeTree {
		r, err := ag.GetTyped(ctx, s, "", x)
		require.NoError(t, err)
		data, err := io.ReadAll(r)
		require.NoError(t, err)
		t.Log(string(data))
		return
	}
	err := ag.WalkTree(ctx, s, x, func(prefix string, tree glfs.TreeEntry) error {
		p := path.Join(prefix, tree.Name)
		spacing := strings.Repeat("  ", strings.Count(prefix, "/"))
		t.Log(spacing, p, tree.FileMode, tree.Ref)
		return nil
	})
	require.NoError(t, err)
}

func PrintFile(t testing.TB, s cadata.Store, x glfs.Ref, p string) {
	ctx := Context(t)
	ref, err := glfs.GetAtPath(ctx, s, x, p)
	require.NoError(t, err)
	data, err := glfs.GetBlobBytes(ctx, s, *ref, 1e9)
	require.NoError(t, err)
	t.Log(string(data))
}

func BlobContains(t testing.TB, s cadata.Store, x glfs.Ref, p string, ss []byte) {
	ctx := Context(t)
	ref, err := glfs.GetAtPath(ctx, s, x, p)
	require.NoError(t, err)
	data, err := glfs.GetBlobBytes(ctx, s, *ref, 1e9)
	require.NoError(t, err)
	require.Contains(t, string(data), string(ss))
}

func LoadFile(t testing.TB, s cadata.Poster, p string) glfs.Ref {
	ctx := Context(t)
	op := glfs.NewAgent()
	f, err := os.Open(p)
	require.NoError(t, err)
	defer f.Close()
	ref, err := op.PostBlob(ctx, s, f)
	require.NoError(t, err)
	return *ref
}

func LoadTarGz(t testing.TB, s cadata.GetPoster, p string, subpath string) glfs.Ref {
	ctx := Context(t)
	ag := glfs.NewAgent()

	f, err := os.Open(p)
	require.NoError(t, err)
	defer f.Close()
	gr, err := gzip.NewReader(f)
	require.NoError(t, err)
	defer gr.Close()
	tr := tar.NewReader(gr)
	ref, err := glfstar.ReadTAR(ctx, ag, s, tr)
	require.NoError(t, err)
	if subpath != "" {
		ref, err = glfs.GetAtPath(ctx, s, *ref, subpath)
		require.NoError(t, err)
	}
	require.NoError(t, err)
	return *ref
}

func PostTree(t testing.TB, s cadata.Store, ents []glfs.TreeEntry) glfs.Ref {
	ctx := Context(t)
	ag := glfs.NewAgent()
	ref, err := ag.PostTreeEntries(ctx, s, ents)
	require.NoError(t, err)
	return *ref
}

func MergeFS(t testing.TB, s cadata.Store, xs ...glfs.Ref) glfs.Ref {
	ctx := Context(t)
	ag := glfs.NewAgent()
	ref, err := ag.Merge(ctx, s, xs...)
	require.NoError(t, err)
	return *ref
}

func PostBlob(t testing.TB, s cadata.Poster, data []byte) glfs.Ref {
	ctx := Context(t)
	ag := glfs.NewAgent()
	ref, err := ag.PostBlob(ctx, s, bytes.NewReader(data))
	require.NoError(t, err)
	return *ref
}

func SymlinkTree(t testing.TB, s cadata.Store, target, newp string) glfs.Ref {
	ctx := Context(t)
	ag := glfs.NewAgent()
	ref, err := ag.PostBlob(ctx, s, strings.NewReader(target))
	require.NoError(t, err)
	ref, err = ag.PostTreeEntries(ctx, s, []glfs.TreeEntry{
		{Name: newp, FileMode: fs.ModeSymlink | 0o644, Ref: *ref},
	})
	require.NoError(t, err)
	return *ref
}
