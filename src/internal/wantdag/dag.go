package wantdag

import (
	"context"
	"fmt"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/exp/streams"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/wantjob"
)

type DAG []Node

func GetDAG(ctx context.Context, s cadata.Getter, ref glfs.Ref) (DAG, error) {
	_, err := glfs.GetTree(ctx, s, ref)
	if err != nil {
		return nil, err
	}
	factsRef, err := glfs.GetAtPath(ctx, s, ref, "facts")
	if err != nil {
		return nil, err
	}
	factTree, err := glfs.GetTree(ctx, s, *factsRef)
	if err != nil {
		return nil, err
	}
	if factsRef.Type != glfs.TypeTree {
		return nil, fmt.Errorf("DAG is missing facts dir")
	}
	nodesRef, err := glfs.GetAtPath(ctx, s, ref, "nodes.json")
	if err != nil {
		return nil, err
	}
	r, err := glfs.GetBlob(ctx, s, *nodesRef)
	if err != nil {
		return nil, err
	}
	nlr := NewNodeListReader(r)
	nodes, err := streams.Collect(ctx, nlr, 1e7)
	if err != nil {
		return nil, err
	}
	var factCount int
	for i, node := range nodes {
		if node.IsFact() {
			ent := factTree.Lookup(nodeName(NodeID(i)))
			if !ent.Ref.Equals(*node.Value) {
				return nil, fmt.Errorf("fact ref does not match node %d", i)
			}
			factCount++
		}
	}
	if len(factTree.Entries) != factCount {
		return nil, fmt.Errorf("too many facts in DAG")
	}
	return nodes, nil
}

func PostDAG(ctx context.Context, s cadata.Poster, x DAG) (*glfs.Ref, error) {
	var factEnts []glfs.TreeEntry
	for i, node := range x {
		if node.IsFact() {
			factEnts = append(factEnts, glfs.TreeEntry{
				Name: nodeName(NodeID(i)),
				Ref:  *node.Value,
			})
		}
	}
	facts, err := glfs.PostTreeEntries(ctx, s, factEnts)
	if err != nil {
		return nil, err
	}

	nlb := NewNodeListBuilder(s)
	for _, node := range x {
		_, err := nlb.Add(node)
		if err != nil {
			return nil, err
		}
	}
	nlRef, err := nlb.Finish(ctx)
	if err != nil {
		return nil, err
	}

	return glfs.PostTreeMap(ctx, s, map[string]glfs.Ref{
		"facts":      *facts,
		"nodes.json": *nlRef,
	})
}

func EditDAG(ctx context.Context, dst cadata.Store, src cadata.Getter, x glfs.Ref, fn func(DAG) (*DAG, error)) (*glfs.Ref, error) {
	dagX, err := GetDAG(ctx, src, x)
	if err != nil {
		return nil, err
	}
	dagY, err := fn(dagX)
	if err != nil {
		return nil, err
	}
	return PostDAG(ctx, dst, *dagY)
}

func nodeName(x NodeID) string {
	return fmt.Sprintf("%016x", x)
}

// Builder performs checks while building a DAG.
type Builder struct {
	nodes []Node
	dst   cadata.Store
}

func NewBuilder(dst cadata.Store) Builder {
	return Builder{dst: dst}
}

func (b *Builder) Fact(ctx context.Context, src cadata.Getter, x glfs.Ref) (NodeID, error) {
	if err := glfstasks.FastSync(ctx, b.dst, src, x); err != nil {
		return 0, err
	}
	nid := len(b.nodes)
	b.nodes = append(b.nodes, Node{
		Value: &x,
	})
	return NodeID(nid), nil
}

func (b *Builder) Derived(ctx context.Context, op wantjob.OpName, inputs []NodeInput) (NodeID, error) {
	nid := NodeID(len(b.nodes))
	for _, input := range inputs {
		if input.Node >= nid {
			return 0, fmt.Errorf("node %d cannot reference node %d", nid, input.Node)
		}
	}
	b.nodes = append(b.nodes, Node{
		Op:     op,
		Inputs: inputs,
	})
	return nid, nil
}

func (b *Builder) Count() uint64 {
	return uint64(len(b.nodes))
}

func (b *Builder) Finish() DAG {
	return b.nodes
}
