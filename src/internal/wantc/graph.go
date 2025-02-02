package wantc

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/wantdag"
)

type (
	NodeID    = wantdag.NodeID
	NodeInput = wantdag.NodeInput
)

type GraphBuilder struct {
	dst   cadata.Store
	index map[[32]byte]wantdag.NodeID
	b     wantdag.Builder
}

func NewGraphBuilder(dst cadata.Store) *GraphBuilder {
	return &GraphBuilder{
		dst:   dst,
		index: map[[32]byte]wantdag.NodeID{},
		b:     wantdag.NewBuilder(dst),
	}
}

func (gb *GraphBuilder) Finish() wantdag.DAG {
	return gb.b.Finish()
}

func (gb *GraphBuilder) Count() uint64 {
	return gb.b.Count()
}

func (gb *GraphBuilder) Derived(ctx context.Context, op wantdag.OpName, inputs []wantdag.NodeInput) (wantdag.NodeID, error) {
	return gb.b.Derived(ctx, op, inputs)
}

func (gb *GraphBuilder) Fact(ctx context.Context, src cadata.Getter, ref glfs.Ref) (wantdag.NodeID, error) {
	return gb.b.Fact(ctx, src, ref)
}

func (gb *GraphBuilder) Expr(ctx context.Context, src cadata.Getter, x Expr) (wantdag.NodeID, error) {
	n, err := gb.expr(ctx, src, x)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (gb *GraphBuilder) expr(ctx context.Context, src cadata.Getter, e Expr) (wantdag.NodeID, error) {
	key := e.Key()
	if n, exists := gb.index[key]; exists {
		return n, nil
	}
	var dcnode wantdag.NodeID
	var err error
	switch x := e.(type) {
	case *value:
		dcnode, err = gb.b.Fact(ctx, src, x.ref)
	case *selection:
		panic(fmt.Sprint("graph builder cannot deal with selections. ", e))
	case *compute:
		dcnode, err = gb.compute(ctx, src, x)
	default:
		panic("empty node")
	}
	if err != nil {
		return 0, err
	}
	gb.index[key] = dcnode
	return dcnode, nil
}

func (gb *GraphBuilder) compute(ctx context.Context, src cadata.Getter, c *compute) (wantdag.NodeID, error) {
	nodeInputs, err := gb.computeInput(ctx, src, c.Inputs)
	if err != nil {
		return 0, err
	}
	return gb.b.Derived(ctx, c.Op, nodeInputs)
}

func (gb *GraphBuilder) computeInput(ctx context.Context, src cadata.Getter, inputs []computeInput) ([]wantdag.NodeInput, error) {
	layers := []wantdag.NodeID{}
	for _, input := range inputs {
		nid, err := gb.expr(ctx, src, input.From)
		if err != nil {
			return nil, err
		}
		nid, err = gb.place(ctx, input.To, nid)
		if err != nil {
			return nil, err
		}
		layers = append(layers, nid)
	}

	switch {
	case len(layers) == 0:
		return []wantdag.NodeInput{}, nil
	case len(layers) == 1:
		return []wantdag.NodeInput{
			{Name: "", Node: layers[0]},
		}, nil
	default:
		return []wantdag.NodeInput{
			{Name: "", Node: gb.mergeLayers(ctx, layers)},
		}, nil
	}
}

func (gb *GraphBuilder) mergeLayers(ctx context.Context, xs []wantdag.NodeID) wantdag.NodeID {
	if len(xs) <= wantdag.MaxNodeInputs {
		return DeriveMerge(&gb.b, xs)
	}
	batchSize := len(xs) / wantdag.MaxNodeInputs
	if len(xs)%wantdag.MaxNodeInputs > 0 {
		batchSize++
	}
	var ys []wantdag.NodeID
	for i := 0; i*batchSize < len(xs); i++ {
		start := i * batchSize
		end := start + batchSize
		if end > len(xs) {
			end = len(xs)
		}
		if start < end {
			n := gb.mergeLayers(ctx, xs[start:end])
			ys = append(ys, n)
		}
	}
	return DeriveMerge(&gb.b, ys)
}

func (gb *GraphBuilder) blob(ctx context.Context, data []byte) (wantdag.NodeID, error) {
	ref, err := glfs.PostBlob(ctx, gb.dst, bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	return gb.b.Fact(ctx, gb.dst, *ref)
}

func (gb *GraphBuilder) emptyTree(ctx context.Context) (wantdag.NodeID, error) {
	ref, err := glfs.PostTreeSlice(ctx, gb.dst, nil)
	if err != nil {
		return 0, err
	}
	return gb.b.Fact(ctx, gb.dst, *ref)
}

func (gb *GraphBuilder) place(ctx context.Context, to string, from wantdag.NodeID) (wantdag.NodeID, error) {
	p := glfs.CleanPath(to)
	if p != "" {
		pathNode, err := gb.blob(ctx, []byte(p))
		if err != nil {
			return 0, err
		}
		from = DerivePlace(&gb.b, from, pathNode)
	}
	return from, nil
}

func inputsNeedMerge(inputs []computeInput) bool {
	if len(inputs) > wantdag.MaxNodeInputs {
		return true
	}
	dirs := map[string]bool{}
	for _, input := range inputs {
		if input.To == "" {
			return true
		}
		to := input.To
		to = strings.Trim(to, "/")
		dir := path.Dir(to)
		if dir == to {
			dir = ""
		}
		if dirs[dir] {
			return true
		}
		dirs[to] = true
	}
	return false
}
