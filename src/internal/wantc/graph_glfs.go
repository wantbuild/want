package wantc

import (
	"context"
	"fmt"
	"strings"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/op/glfsops"
	"wantbuild.io/want/src/internal/wantdag"
	"wantbuild.io/want/src/wantjob"
)

func mustDerived(gb *wantdag.Builder, prefix, op wantjob.OpName, ins []wantdag.NodeInput) NodeID {
	ctx := context.Background()
	nid, err := gb.Derived(ctx, prefix+wantjob.OpName(".")+op, ins)
	if err != nil {
		panic(err)
	}
	return nid
}

func DeriveMerge(g *wantdag.Builder, layers []NodeID) NodeID {
	if len(layers) > wantdag.MaxNodeInputs {
		// TODO: recurse here
		panic(len(layers))
	}
	var inputs []NodeInput
	for i, layer := range layers {
		inputs = append(inputs, NodeInput{
			Name: fmt.Sprintf("%02x", i),
			Node: layer,
		})
	}
	return mustDerived(g, "glfs", glfsops.OpMerge, inputs)
}

func DerivePlace(g *wantdag.Builder, x, path NodeID) NodeID {
	return mustDerived(g, "glfs", glfsops.OpPlace, []NodeInput{
		{Name: "x", Node: x},
		{Name: "path", Node: path},
	})
}

func FactString(ctx context.Context, gb *wantdag.Builder, s cadata.GetPoster, p string) wantdag.NodeID {
	ref, err := glfs.PostBlob(ctx, s, strings.NewReader(p))
	if err != nil {
		panic(err)
	}
	nid, err := gb.Fact(ctx, s, *ref)
	if err != nil {
		panic(err)
	}
	return nid
}

func FactTree(ctx context.Context, gb *wantdag.Builder, dst cadata.PostExister, src cadata.Getter, ents []glfs.TreeEntry) wantdag.NodeID {
	ref, err := glfs.PostTreeSlice(ctx, dst, ents)
	if err != nil {
		panic(err)
	}
	nid, err := gb.Fact(ctx, src, *ref)
	if err != nil {
		panic(err)
	}
	return nid
}
