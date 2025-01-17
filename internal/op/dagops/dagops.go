package dagops

import (
	"fmt"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"
	"wantbuild.io/want/internal/glfstasks"
	"wantbuild.io/want/internal/wantdag"
	"wantbuild.io/want/lib/wantjob"
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

type Executor struct{}

func (e Executor) Execute(jc *wantjob.Ctx, dst cadata.Store, src cadata.Getter, x wantjob.Task) ([]byte, error) {
	switch x.Op {
	case OpExecAll:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			return e.ExecAll(jc, dst, src, x)
		})
	case OpExecLast:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			return e.ExecLast(jc, dst, src, x)
		})
	case OpPickLastValue:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			return e.PickLast(jc, src, x)
		})
	default:
		return nil, wantjob.NewErrUnknownOperator(x.Op)
	}
}

func (e Executor) ExecAll(jc *wantjob.Ctx, dst cadata.GetPoster, s cadata.Getter, ref glfs.Ref) (*glfs.Ref, error) {
	ctx := jc.Context()
	dag, err := wantdag.GetDAG(ctx, s, ref)
	if err != nil {
		return nil, err
	}
	e2 := wantdag.NewSerialExec()
	nrs, err := e2.ExecAll(jc, dst, s, *dag)
	if err != nil {
		return nil, err
	}
	return wantdag.PostNodeResults(ctx, dst, nrs)
}

func (e Executor) PickLast(jc *wantjob.Ctx, s cadata.Getter, ref glfs.Ref) (*glfs.Ref, error) {
	ctx := jc.Context()
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
	return glfstasks.ParseGLFSRef(res.Data)
}

func (e Executor) ExecLast(jc *wantjob.Ctx, dst cadata.GetPoster, s cadata.Getter, ref glfs.Ref) (*glfs.Ref, error) {
	ctx := jc.Context()
	dag, err := wantdag.GetDAG(ctx, s, ref)
	if err != nil {
		return nil, err
	}
	e2 := wantdag.NewSerialExec()
	nrs, err := e2.ExecAll(jc, dst, s, *dag)
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
	return glfstasks.ParseGLFSRef(res.Data)
}
