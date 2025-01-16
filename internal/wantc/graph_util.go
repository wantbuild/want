package wantc

import (
	"context"
	"fmt"
	"strings"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/op/glfsops"
	"wantbuild.io/want/internal/wantdag"
	"wantbuild.io/want/internal/wantjob"
)

type (
	NodeID    = wantdag.NodeID
	NodeInput = wantdag.NodeInput
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

func DerivePick(g *wantdag.Builder, x, path NodeID) wantdag.NodeID {
	return mustDerived(g, "glfs", glfsops.OpPick, []wantdag.NodeInput{
		{Name: "x", Node: x},
		{Name: "path", Node: path},
	})
}

func DerivePlace(g *wantdag.Builder, x, path NodeID) NodeID {
	return mustDerived(g, "glfs", glfsops.OpPlace, []NodeInput{
		{Name: "x", Node: x},
		{Name: "path", Node: path},
	})
}

func DeriveFilter(g *wantdag.Builder, x, filter NodeID) NodeID {
	return mustDerived(g, "glfs", glfsops.OpFilter, []NodeInput{
		{Name: "x", Node: x},
		{Name: "filter", Node: filter},
	})
}

func DeriveChmod(g *wantdag.Builder, x, path NodeID) NodeID {
	return mustDerived(g, "glfs", glfsops.OpChmod, []NodeInput{
		{Name: "path", Node: path},
		{Name: "x", Node: x},
	})
}

func DeriveDiff(g *wantdag.Builder, left, right NodeID) NodeID {
	return mustDerived(g, "glfs", glfsops.OpDiff, []NodeInput{
		{Name: "left", Node: left},
		{Name: "right", Node: right},
	})
}

func DeriveAssert(ctx context.Context, s cadata.GetPoster, gb *wantdag.Builder, x wantdag.NodeID, ac glfsops.AssertChecks) wantdag.NodeID {
	inputs := []NodeInput{
		{Name: "x", Node: x},
	}
	if ac.SubsetOf != nil {
		inputs = append(inputs, NodeInput{Name: "subsetOf", Node: *ac.SubsetOf})
	}
	if ac.Message != "" {
		nid := FactString(ctx, gb, s, ac.Message)
		inputs = append(inputs, NodeInput{Name: "message", Node: nid})
	}
	return mustDerived(gb, "glfs", glfsops.OpAssert, inputs)
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

func FactTree(ctx context.Context, gb *wantdag.Builder, s cadata.GetPoster, ents []glfs.TreeEntry) wantdag.NodeID {
	ref, err := glfs.PostTreeEntries(ctx, s, ents)
	if err != nil {
		panic(err)
	}
	nid, err := gb.Fact(ctx, s, *ref)
	if err != nil {
		panic(err)
	}
	return nid
}
