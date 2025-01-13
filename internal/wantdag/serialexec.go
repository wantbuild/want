package wantdag

import (
	"context"
	"fmt"

	"github.com/blobcache/glfs"
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

func (e *SerialExec) Execute(ctx context.Context, jc *wantjob.JobCtx, s cadata.Getter, x DAG) ([]glfs.Ref, error) {
	nodeResults := make([]glfs.Ref, len(x.Nodes))
	resolve := func(nid NodeID) *glfs.Ref {
		out := nodeResults[nid]
		if out.CID.IsZero() {
			return nil
		}
		return &out
	}
	for i, n := range x.Nodes {
		if n.IsFact() {
			nodeResults[i] = *n.Value
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
			return nil, fmt.Errorf("critical error in node %d (%w). stopping", i, err)
		}
		nodeResults[i] = *out
	}
	return nodeResults, nil
}
