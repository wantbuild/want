package dagops

import (
	"blobcache.io/blobcache/src/bcsdk"
	"blobcache.io/glfs"
	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/internal/wantdag"
	"wantbuild.io/want/src/wantjob"
)

const (
	// OpExec evaluates a subgraph.
	OpExecLast = wantjob.OpName("execLast")
)

var _ wantjob.Executor = &Executor{}

type Executor struct{}

func (e Executor) Execute(jc wantjob.Ctx, src bcsdk.RO, x wantjob.Task) wantjob.Result {
	switch x.Op {
	case OpExecLast:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			return e.ExecLast(jc, src, x)
		})
	default:
		return *wantjob.Result_ErrExec(wantjob.NewErrUnknownOperator(x.Op))
	}
}

func (e Executor) ExecLast(jc wantjob.Ctx, s bcsdk.RO, ref glfs.Ref) (*glfs.Ref, error) {
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
	return glfstasks.ParseGLFSRef(res.Root)
}
