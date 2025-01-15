package wantdag

import (
	"context"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/exp/slices2"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/wantjob"
)

// SerialExec is a serial executor for graphs
type SerialExec struct {
	store cadata.GetPoster
}

func NewSerialExec(s cadata.GetPoster) *SerialExec {
	return &SerialExec{
		store: s,
	}
}

func (e *SerialExec) GetStore() cadata.Getter {
	return e.store
}

func (e *SerialExec) Execute(ctx context.Context, jc *wantjob.Ctx, s cadata.Getter, x DAG) ([]glfs.Ref, error) {
	nodeResults := make([]*wantjob.Result, len(x.Nodes))
	resolve := func(nid NodeID) *glfs.Ref {
		res := nodeResults[nid]
		if res == nil {
			return nil
		}
		return &res.Data
	}
	for i, n := range x.Nodes {
		if n.IsFact() {
			nodeResults[i] = &wantjob.Result{Data: *n.Value}
			continue
		}
		input, err := PrepareInput(ctx, s, e.store, n, resolve)
		if err != nil {
			return nil, err
		}
		out, err := wantjob.Do(ctx, jc, wantjob.Task{
			Op:    n.Op,
			Input: *input,
		})
		if err != nil {
			return nil, err
		}
		nodeResults[i] = out
	}
	return slices2.Map(nodeResults, func(res *wantjob.Result) glfs.Ref {
		return res.Data
	}), nil
}
