package wantdag

import (
	"context"
	"fmt"
	"io/fs"
	"strconv"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"
)

type Resolver = func(NodeID) *glfs.Ref

// PrepareInput prepares the input for a node.
func PrepareInput(ctx context.Context, s cadata.Getter, dst cadata.Poster, n Node, getResult Resolver) (*glfs.Ref, error) {
	ents := []glfs.TreeEntry{}
	for _, in := range n.Inputs {
		ref := getResult(in.Node)
		if ref.CID.IsZero() {
			return nil, fmt.Errorf("node input %q from %d is not available", in.Name, in.Node)
		}
		mode := InputFileMode
		if ref.Type == glfs.TypeTree {
			mode |= fs.ModeDir
		}
		ents = append(ents, glfs.TreeEntry{
			Name:     in.Name,
			FileMode: mode,
			Ref:      *ref,
		})
	}
	if len(ents) == 1 && ents[0].Name == "" {
		return &ents[0].Ref, nil
	}
	return glfs.PostTreeEntries(ctx, dst, ents)
}

func PostNodeResults(ctx context.Context, s cadata.Poster, results []glfs.Ref) (*glfs.Ref, error) {
	var ents []glfs.TreeEntry
	for i, out := range results {
		ents = append(ents, glfs.TreeEntry{
			Name: nodeName(NodeID(i)),
			Ref:  out,
		})
	}
	return glfs.PostTreeEntries(ctx, s, ents)
}

func GetNodeResults(ctx context.Context, s cadata.Getter, ref glfs.Ref) ([]glfs.Ref, error) {
	tree, err := glfs.GetTree(ctx, s, ref)
	if err != nil {
		return nil, err
	}
	var ret []glfs.Ref
	for i, ent := range tree.Entries {
		n, err := strconv.ParseUint(ent.Name, 16, 64)
		if err != nil {
			return nil, err
		}
		if NodeID(i) != NodeID(n) {
			return nil, fmt.Errorf("missing result for %d", n)
		}
		ret = append(ret, ent.Ref)
	}
	return ret, nil
}
