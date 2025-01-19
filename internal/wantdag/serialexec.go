package wantdag

import (
	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/glfstasks"
	"wantbuild.io/want/internal/stores"
	"wantbuild.io/want/lib/wantjob"
)

func ExecLast(jc *wantjob.Ctx, dst cadata.Store, s cadata.Getter, x DAG) (wantjob.Result, error) {
	// TODO: lazy execution
	nrs, err := ExecAll(jc, dst, s, x)
	if err != nil {
		return wantjob.Result{}, err
	}
	return nrs[len(nrs)-1], nil
}

// ExecAll executes all nodes in the DAG, and returns the result of returning each.
func ExecAll(jc *wantjob.Ctx, dst cadata.Store, s cadata.Getter, x DAG) ([]wantjob.Result, error) {
	ctx := jc.Context()
	nodeStores := make([]cadata.Getter, len(x.Nodes))
	nodeResults := make([]wantjob.Result, len(x.Nodes))
	resolve := func(nid NodeID) wantjob.Result {
		return nodeResults[nid]
	}
	scratch := stores.NewMem()
	for i, n := range x.Nodes {
		var outRef *glfs.Ref
		union := stores.Union{s, dst, scratch}
		switch {
		case n.IsFact():
			nodeResults[i] = *glfstasks.Success(*n.Value)
			nodeStores[i] = s
			outRef = n.Value
		case n.IsDerived():
			input, err := PrepareInput(ctx, s, scratch, n, resolve)
			if err != nil {
				return nil, err
			}
			for _, in := range n.Inputs {
				union = append(union, nodeStores[in.Node])
			}
			out, outSrc, err := wantjob.Do(ctx, jc, union, wantjob.Task{
				Op:    n.Op,
				Input: glfstasks.MarshalGLFSRef(*input),
			})
			union = append(union, outSrc)
			if err != nil {
				return nil, err
			}
			if err := out.Err(); err != nil {
				jc.Infof("ERROR: %v", err)
			}
			nodeResults[i] = *out
			nodeStores[i] = outSrc
			outRef, _ = glfstasks.ParseGLFSRef(out.Data)
		}
		if outRef != nil {
			if err := glfs.Sync(ctx, dst, union, *outRef); err != nil {
				return nil, err
			}
		}
	}
	return nodeResults, nil
}
