package dagops

import (
	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"
	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/internal/wantdag"
	"wantbuild.io/want/src/wantjob"
)

const (
	// OpExec evaluates a subgraph.
	OpExecAll  = wantjob.OpName("execAll")
	OpExecLast = wantjob.OpName("execLast")
)

var _ wantjob.Executor = &Executor{}

type Executor struct{}

func (e Executor) Execute(jc wantjob.Ctx, src cadata.Getter, x wantjob.Task) ([]byte, error) {
	switch x.Op {
	case OpExecAll:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			return e.ExecAll(jc, src, x)
		})
	case OpExecLast:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			return e.ExecLast(jc, src, x)
		})
	default:
		return nil, wantjob.NewErrUnknownOperator(x.Op)
	}
}

func (e Executor) ExecAll(jc wantjob.Ctx, s cadata.Getter, ref glfs.Ref) (*glfs.Ref, error) {
	defer jc.InfoSpan("dag.execAll")()
	ctx := jc.Context
	dag, err := wantdag.GetDAG(ctx, s, ref)
	if err != nil {
		return nil, err
	}
	nrs, err := wantdag.ParallelExecAll(jc, s, dag)
	if err != nil {
		return nil, err
	}
	return wantdag.PostNodeResults(ctx, jc.Dst, nrs)
}

func (e Executor) ExecLast(jc wantjob.Ctx, s cadata.Getter, ref glfs.Ref) (*glfs.Ref, error) {
	ctx := jc.Context
	dag, err := wantdag.GetDAG(ctx, s, ref)
	if err != nil {
		return nil, err
	}
	res, err := wantdag.ParallelExecLast(jc, s, dag)
	if err != nil {
		return nil, err
	}
	if err := res.Err(); err != nil {
		return nil, err
	}
	return glfstasks.ParseGLFSRef(res.Data)
}
