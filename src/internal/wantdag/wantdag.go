package wantdag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"strconv"

	"blobcache.io/glfs"
	"go.brendoncarroll.net/state/cadata"
	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/wantjob"
)

type Resolver = func(NodeID) wantjob.Result

// PrepareInput prepares the input for a node.
func PrepareInput(ctx context.Context, dst cadata.PostExister, src cadata.Getter, ins []NodeInput, getResult Resolver) (*glfs.Ref, error) {
	ents := []glfs.TreeEntry{}
	for _, in := range ins {
		res := getResult(in.Node)
		if err := res.Err(); err != nil {
			return nil, fmt.Errorf("upstream node %d errored: %v", in.Node, err)
		}
		ref, err := glfstasks.ParseGLFSRef(res.Root)
		if err != nil {
			return nil, fmt.Errorf("cannot convert job output (%s) to GLFS Ref: %v", res.Root, err)
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
	return glfs.PostTreeSlice(ctx, dst, ents)
}

func PostNodeResults(ctx context.Context, s cadata.PostExister, results []wantjob.Result) (*glfs.Ref, error) {
	var ents []glfs.TreeEntry
	for i, out := range results {
		data, err := json.Marshal(out)
		if err != nil {
			return nil, err
		}
		ref, err := glfs.PostBlob(ctx, s, bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		ents = append(ents, glfs.TreeEntry{
			Name: nodeName(NodeID(i)),
			Ref:  *ref,
		})
	}
	return glfs.PostTreeSlice(ctx, s, ents)
}

func GetNodeResults(ctx context.Context, s cadata.Getter, ref glfs.Ref) ([]wantjob.Result, error) {
	tree, err := glfs.GetTreeSlice(ctx, s, ref, 1e6)
	if err != nil {
		return nil, err
	}
	var ret []wantjob.Result
	for i, ent := range tree {
		n, err := strconv.ParseUint(ent.Name, 16, 64)
		if err != nil {
			return nil, err
		}
		if NodeID(i) != NodeID(n) {
			return nil, fmt.Errorf("missing result for %d", n)
		}
		data, err := glfs.GetBlobBytes(ctx, s, ent.Ref, 1024)
		if err != nil {
			return nil, err
		}
		var res wantjob.Result
		if err := json.Unmarshal(data, &res); err != nil {
			return nil, err
		}
		ret = append(ret, res)
	}
	return ret, nil
}
