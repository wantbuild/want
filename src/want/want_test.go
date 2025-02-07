package want

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/blobcache/glfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/internal/testutil"
	"wantbuild.io/want/src/internal/wantdb"
)

func TestInit(t *testing.T) {
	t.SkipNow() // This dowloads a bunch of stuff from the internet

	ctx := testutil.Context(t)
	dir := t.TempDir()
	sys := New(dir, runtime.GOMAXPROCS(0))
	require.NoError(t, sys.Init(ctx))
}

func TestEvalNoRepo(t *testing.T) {
	ctx := testutil.Context(t)
	db := wantdb.NewMemory()
	require.NoError(t, wantdb.Setup(ctx, db))
	sys := New(t.TempDir(), runtime.GOMAXPROCS(0))
	require.NoError(t, sys.init(ctx, false)) // init without install
	s := stores.NewMem()

	tcs := []struct {
		Name string
		I    string
		O    glfs.Ref
	}{
		{
			Name: "blob/hello world",
			I:    `want.blob("hello world")`,
			O:    testutil.PostBlob(t, s, []byte("hello world")),
		},
		{
			Name: "blob/empty",
			I:    `want.blob("")`,
			O:    testutil.PostBlob(t, s, []byte("")),
		},
		{
			Name: "tree/empty",
			I:    `want.tree({})`,
			O:    testutil.PostTree(t, s, nil),
		},
		{
			Name: "tree/3blobs",
			I: `want.tree({
				"k1": want.treeEntry("0644", want.blob("v1")),
				"k2": want.treeEntry("0644", want.blob("v2")),
				"k3": want.treeEntry("0644", want.blob("v3")),
			})`,
			O: testutil.PostTree(t, s, []glfs.TreeEntry{
				{Name: "k1", FileMode: 0o644, Ref: testutil.PostBlob(t, s, []byte("v1"))},
				{Name: "k2", FileMode: 0o644, Ref: testutil.PostBlob(t, s, []byte("v2"))},
				{Name: "k3", FileMode: 0o644, Ref: testutil.PostBlob(t, s, []byte("v3"))},
			}),
		},
		{
			Name: "pass3",
			I: `want.pass([
				want.input("a.txt", want.blob("foo")),
				want.input("b.d",  want.tree({})),
				want.input("c.txt", want.blob("bar")),
			])`,
			O: testutil.PostTree(t, s, []glfs.TreeEntry{
				{Name: "a.txt", FileMode: 0o777, Ref: testutil.PostBlob(t, s, []byte("foo"))},
				{Name: "b.d", FileMode: 0o777, Ref: testutil.PostTree(t, s, nil)},
				{Name: "c.txt", FileMode: 0o777, Ref: testutil.PostBlob(t, s, []byte("bar"))},
			}),
		},
	}
	for i, tc := range tcs {
		tc := tc
		in := `local want = import "want";` + "\n" + tc.I
		t.Run(fmt.Sprintf("%d-%s", i, tc.Name), func(t *testing.T) {
			out, src, err := sys.Eval(ctx, db, nil, "", []byte(in))
			require.NoError(t, err)
			if !assert.Equal(t, tc.O, *out) {
				t.Log("EXPECTED:")
				testutil.PrintFS(t, s, tc.O)
				t.Log("ACTUAL:")
				testutil.PrintFS(t, src, *out)
			}
			require.NoError(t, glfs.WalkRefs(ctx, src, *out, func(ref glfs.Ref) error { return nil }))
		})
	}
}
