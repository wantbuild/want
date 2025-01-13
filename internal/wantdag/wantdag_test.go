package wantdag

import (
	"testing"

	"github.com/stretchr/testify/require"

	"wantbuild.io/want/internal/stores"
	"wantbuild.io/want/internal/testutil"
)

func TestDAGPostGet(t *testing.T) {
	ctx := testutil.Context(t)
	s := stores.NewMem()
	b := NewBuilder(s)
	for i := 0; i < 10; i++ {
		_, err := b.Fact(ctx, s, testutil.PostFS(t, s, nil))
		require.NoError(t, err)
	}
	for i := 0; i < 10; i++ {
		_, err := b.Derived(ctx, OpName("test"), []NodeInput{
			{Name: "a", Node: NodeID(i)},
			{Name: "b", Node: NodeID(i + 9)},
		})
		require.NoError(t, err)
	}
	x := b.Finish()
	require.Len(t, x.Nodes, 20)

	ref, err := PostDAG(ctx, s, x)
	require.NoError(t, err)

	y, err := GetDAG(ctx, s, *ref)
	require.NoError(t, err)
	require.Equal(t, x, *y)
}
