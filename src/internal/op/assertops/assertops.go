package assertops

import (
	"blobcache.io/glfs"
	"go.brendoncarroll.net/state/cadata"
	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/wantjob"
)

const OpAll = wantjob.OpName("all")

var _ wantjob.Executor = Executor{}

type Executor struct{}

func (e Executor) Execute(jc wantjob.Ctx, src cadata.Getter, x wantjob.Task) wantjob.Result {
	ctx := jc.Context
	switch x.Op {
	case OpAll:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			at, err := GetAssertTask(ctx, src, x)
			if err != nil {
				return nil, err
			}
			ref, err := AssertAll(ctx, jc.Dst, src, *at)
			if err != nil {
				return nil, err
			}
			if err := glfstasks.FastSync(ctx, jc.Dst, src, *ref); err != nil {
				return nil, err
			}
			return ref, nil
		})
	default:
		return *wantjob.Result_ErrExec(wantjob.NewErrUnknownOperator(x.Op))
	}
}
