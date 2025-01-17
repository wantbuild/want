package wantdag

import (
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/glfstasks"
	"wantbuild.io/want/internal/stores"
	"wantbuild.io/want/lib/wantjob"
)

// SerialExec is a serial executor for graphs
type SerialExec struct {
}

func NewSerialExec() *SerialExec {
	return &SerialExec{}
}

func (e *SerialExec) ExecAll(jc *wantjob.Ctx, dst cadata.GetPoster, s cadata.Getter, x DAG) ([]wantjob.Result, error) {
	ctx := jc.Context()
	nodeResults := make([]wantjob.Result, len(x.Nodes))
	resolve := func(nid NodeID) wantjob.Result {
		return nodeResults[nid]
	}
	for i, n := range x.Nodes {
		if n.IsFact() {
			nodeResults[i] = *glfstasks.Success(*n.Value)
			continue
		}
		input, err := PrepareInput(ctx, s, dst, n, resolve)
		if err != nil {
			return nil, err
		}
		out, err := wantjob.Do(ctx, jc, stores.Union{s, dst}, wantjob.Task{
			Op:    n.Op,
			Input: glfstasks.MarshalGLFSRef(*input),
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
