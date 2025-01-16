package wantdag

import (
	"context"
	"encoding/json"

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

func (e *SerialExec) Execute(ctx context.Context, jc *wantjob.Ctx, s cadata.Getter, x DAG) ([]wantjob.Result, error) {
	nodeResults := make([]wantjob.Result, len(x.Nodes))
	resolve := func(nid NodeID) wantjob.Result {
		return nodeResults[nid]
	}
	for i, n := range x.Nodes {
		if n.IsFact() {
			data, err := json.Marshal(*n.Value)
			if err != nil {
				return nil, err
			}
			nodeResults[i] = wantjob.Result{Data: data}
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
		if err := out.Err(); err != nil {
			jc.Infof("ERROR: %v", err)
		}
		nodeResults[i] = *out
	}
	return nodeResults, nil
}
