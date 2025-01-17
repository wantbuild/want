package wantc

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"wantbuild.io/want/internal/stores"
	"wantbuild.io/want/internal/stringsets"
	"wantbuild.io/want/internal/testutil"
	"wantbuild.io/want/lib/wantcfg"
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
		x2 := makePathSet(y)
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
	require.Len(t, dag.Nodes, 1)
	require.Equal(t, *dag.Nodes[0].Value, r1)
}

func TestSnippets(t *testing.T) {
	tcs := []struct {
		I string
	}{
		{`local want = import "want";
			want.blob("hello world")
		`},
		{`local want = import "want";
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
