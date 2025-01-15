package dagops

import (
	"context"
	"fmt"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"
	"wantbuild.io/want/internal/wantc"
	"wantbuild.io/want/internal/wantdag"
	"wantbuild.io/want/internal/wantjob"
)

// OpName uniquely identifies an Operation
type OpName = wantjob.OpName

const (
	// OpExec evaluates a subgraph.
	OpExec     = OpName("exec")
	OpExecLast = OpName("execLast")
	// OpPickLast takes as input a result set, and evaluates to the value of the result, or errors if it was not successful.
	OpPickLastValue = OpName("pickLast")
)

var _ wantjob.Executor = &Executor{}

type Executor struct {
	s    cadata.Store
	glfs *glfs.Agent
	c    *wantc.Compiler
}

func NewExecutor(s cadata.Store) Executor {
	return Executor{
		s:    s,
		glfs: glfs.NewAgent(),
		c:    wantc.NewCompiler(s),
	}
}

func (e Executor) Compute(ctx context.Context, jc *wantjob.Ctx, src cadata.Getter, x wantjob.Task) (*glfs.Ref, error) {
	switch x.Op {
	case OpExec:
		return e.Exec(ctx, jc, src, x.Input)
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

func (e Executor) Exec(ctx context.Context, jc *wantjob.Ctx, s cadata.Getter, ref glfs.Ref) (*glfs.Ref, error) {
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
		return nil, fmt.Errorf("empty node result list")
	}
	return &nrs[len(nrs)-1], nil
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
	if len(nrs) < 1 {
		return nil, fmt.Errorf("empty node results")
	}
	return &nrs[len(nrs)-1], nil
}
