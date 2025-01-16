package dagops

import (
	"context"
	"fmt"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"
	"wantbuild.io/want/internal/wantdag"
	"wantbuild.io/want/internal/wantjob"
)

// OpName uniquely identifies an Operation
type OpName = wantjob.OpName

const (
	// OpExec evaluates a subgraph.
	OpExecAll  = OpName("execAll")
	OpExecLast = OpName("execLast")
	// OpPickLast takes as input a result set, and evaluates to the value of the result, or errors if it was not successful.
	OpPickLastValue = OpName("pickLast")
)

var _ wantjob.Executor = &Executor{}

type Executor struct {
	s cadata.GetPoster
}

func NewExecutor(s cadata.GetPoster) Executor {
	return Executor{
		s: s,
	}
}

func (e Executor) Compute(ctx context.Context, jc *wantjob.Ctx, src cadata.Getter, x wantjob.Task) (*glfs.Ref, error) {
	switch x.Op {
	case OpExecAll:
		return e.ExecAll(ctx, jc, src, x.Input)
	case OpExecLast:
		return e.ExecLast(ctx, jc, src, x.Input)
	case OpPickLastValue:
		return e.PickLast(ctx, jc, src, x.Input)
	default:
		return nil, wantjob.NewErrUnknownOperator(x.Op)
	}
}

func (e Executor) GetStore() cadata.Getter {
	return e.s
}

func (e Executor) ExecAll(ctx context.Context, jc *wantjob.Ctx, s cadata.Getter, ref glfs.Ref) (*glfs.Ref, error) {
	dag, err := wantdag.GetDAG(ctx, s, ref)
	if err != nil {
		return nil, err
	}
	e2 := wantdag.NewSerialExec(e.s)
	nrs, err := e2.Execute(ctx, jc, s, *dag)
	if err != nil {
		return nil, err
	}
	return wantdag.PostNodeResults(ctx, e.s, nrs)
}

func (e Executor) PickLast(ctx context.Context, _ *wantjob.Ctx, s cadata.Getter, ref glfs.Ref) (*glfs.Ref, error) {
	nrs, err := wantdag.GetNodeResults(ctx, s, ref)
	if err != nil {
		return nil, err
	}
	if len(nrs) == 0 {
		return nil, fmt.Errorf("empty node results")
	}
	res := nrs[len(nrs)-1]
	if err := res.Err(); err != nil {
		return nil, err
	}
	return res.AsGLFS()
}

func (e Executor) ExecLast(ctx context.Context, jc *wantjob.Ctx, s cadata.Getter, ref glfs.Ref) (*glfs.Ref, error) {
	dag, err := wantdag.GetDAG(ctx, s, ref)
	if err != nil {
		return nil, err
	}
	e2 := wantdag.NewSerialExec(e.s)
	nrs, err := e2.Execute(ctx, jc, s, *dag)
	if err != nil {
		return nil, err
	}
	if len(nrs) == 0 {
		return nil, fmt.Errorf("empty node results")
	}
	res := nrs[len(nrs)-1]
	if err := res.Err(); err != nil {
		return nil, err
	}
	return res.AsGLFS()
}
