package wantc

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/blobcache/glfs"
	"github.com/stretchr/testify/require"

	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/internal/stringsets"
	"wantbuild.io/want/src/internal/testutil"
	"wantbuild.io/want/src/wantcfg"
)

func TestConvertSet(t *testing.T) {
	inputs := []wantcfg.PathSet{
		wantcfg.Prefix("abc"),
		wantcfg.Suffix("abc"),
		wantcfg.Subtract(wantcfg.Prefix("abc"), wantcfg.Suffix("xyz")),
		wantcfg.Union(wantcfg.Prefix("abc"), wantcfg.Prefix("123")),
	}

	for _, x := range inputs {
		y := SetFromQuery("", x)
		x2 := stringsets.ToPathSet(y)
		require.Equal(t, x, x2)
	}
}

func TestVFS(t *testing.T) {
	v := VFS{}
	require.NoError(t, v.Add(VFSEntry{
		K: stringsets.Prefix("abc"),
	}))
	require.NoError(t, v.Add(VFSEntry{
		K: stringsets.Prefix("123"),
	}))
	ents := v.Get(stringsets.Suffix("xyz"))
	require.Len(t, ents, 2)
}

func TestPathFrom(t *testing.T) {
	tcs := []struct {
		From, Target string
		Output       string
	}{
		{"dir/a.libsonnet", "./target.txt", "dir/target.txt"},
		{"a/b/c.libsonnet", "./c1/target.txt", "a/b/c1/target.txt"},
	}
	for _, tc := range tcs {
		actual := PathFrom(tc.From, tc.Target)
		require.Equal(t, tc.Output, actual)
	}
}

func TestRef(t *testing.T) {
	ctx := testutil.Context(t)
	s := stores.NewMem()
	r1 := testutil.PostBlob(t, s, []byte("hello world"))
	data, err := json.Marshal(map[string]any{
		"ref": r1,
	})
	require.NoError(t, err)
	t.Log(string(data))

	c := NewCompiler()
	dag, err := c.CompileSnippet(ctx, s, s, data)
	require.NoError(t, err)
	require.NoError(t, err)
	require.Len(t, dag, 1)
	require.Equal(t, *dag[0].Value, r1)
}

func TestSnippets(t *testing.T) {
	tcs := []struct {
		I string
	}{
		{`local want = import "@want";
			want.blob("hello world")
		`},
		{`local want = import "@want";
			want.tree({
				"k1": want.treeEntry("0644", want.blob("hello")),
			})
		`},
	}
	for i, tc := range tcs {
		tc := tc
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			ctx := testutil.Context(t)
			s := stores.NewMem()
			c := NewCompiler()
			_, err := c.CompileSnippet(ctx, s, stores.Union{}, []byte(tc.I))
			require.NoError(t, err)
		})
	}
}

func TestCompile(t *testing.T) {
	src := stores.NewMem()
	tcs := []struct {
		// Name of the test (optional)
		Name string
		// Module is the mod
		Module glfs.Ref
		Deps   map[ExprID]glfs.Ref

		// If not nil, check that the actual targets match these expected targets
		Targets []Target
		// If not nil, check that the actual Known matches this Ref
		Known *glfs.Ref
		// Check that the error matches
		Err error
	}{
		{
			Name: "Minimal",
			Module: testutil.PostFSStr(t, src, map[string]string{
				"WANT": "{}",
			}),
			Known: ptrTo(testutil.PostFSStr(t, src, map[string]string{
				"WANT": "{}",
			})),
		},
		{
			Name: "GROUND",
			Module: testutil.PostFSStr(t, src, map[string]string{
				"WANT": `{namespace: {want: {blob: importstr "@want"} } }`,
				"test.want": `
					local want = import "@want";
					want.select(GROUND, want.unit("WANT"))
				`,
			}),
			Deps: map[ExprID]glfs.Ref{
				NewExprID(wantcfg.Expr{Blob: ptrTo(LibWant())}): testutil.PostBlob(t, src, []byte(LibWant())),
			},
		},
		{
			Name: "DERIVED",
			Module: testutil.PostFSStr(t, src, map[string]string{
				"WANT":     `{namespace: {want: {blob: importstr "@want"} } }`,
				"test.txt": "foo",
				"test.want": `
					local want = import "@want";
					want.select(DERIVED, want.unit("test.text"))
				`,
			}),
			Deps: map[ExprID]glfs.Ref{
				NewExprID(wantcfg.Expr{Blob: ptrTo(LibWant())}): testutil.PostBlob(t, src, []byte(LibWant())),
			},

			Known: ptrTo(testutil.PostFSStr(t, src, map[string]string{
				"WANT":     `{namespace: {want: {blob: importstr "@want"} } }`,
				"test.txt": "foo",
			})),
		},
		{
			Name: "ErrMissingDep",
			Module: testutil.PostFSStr(t, src, map[string]string{
				"WANT": `
				local want = import "@want";
				{
					"namespace": {
						"name1": want.blob("{}"),
					},
				}`,
			}),
			Err: ErrMissingDep{Name: "name1"},
		},
		{
			Name: "2Modules1Dep",
			Module: testutil.PostFSStr(t, src, map[string]string{
				"WANT": `local want = import "@want";
				{
					"namespace": {
						"want": want.blob(importstr "@want"),
					},
				}`,
				"sub1/WANT": `local want = import "@want";
				{
					"namespace": {
						"want": want.blob(importstr "@want"),
					},
				}`,
			}),
			Deps: map[ExprID]glfs.Ref{
				NewExprID(wantcfg.Expr{Blob: ptrTo(LibWant())}): testutil.PostBlob(t, src, []byte(LibWant())),
			},
		},
	}

	for i, tc := range tcs {
		t.Run(fmt.Sprintf("%d-%s", i, tc.Name), func(t *testing.T) {
			ctx := testutil.Context(t)
			c := NewCompiler()
			dst := stores.NewMem()
			plan, err := c.Compile(ctx, dst, src, CompileTask{Module: tc.Module, Deps: tc.Deps})
			if tc.Err != nil {
				require.ErrorIs(t, err, tc.Err)
			} else {
				require.NoError(t, err)
			}
			if tc.Known != nil {
				testutil.EqualFS(t, stores.Union{dst, src}, *tc.Known, plan.Known)
			}
		})
	}
}

func ptrTo[T any](x T) *T {
	return &x
}
