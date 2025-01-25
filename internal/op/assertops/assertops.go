package assertops

import (
	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"
	"wantbuild.io/want/internal/glfstasks"
	"wantbuild.io/want/lib/wantjob"
)

const OpAll = wantjob.OpName("all")

var _ wantjob.Executor = Executor{}

type Executor struct{}

func (e Executor) Execute(jc wantjob.Ctx, src cadata.Getter, x wantjob.Task) ([]byte, error) {
	ctx := jc.Context
	switch x.Op {
	case OpAll:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			at, err := GetAssertTask(ctx, src, x)
			if err != nil {
				return nil, err
			}
			return AssertAll(ctx, jc.Dst, src, *at)
		})
	default:
		return nil, wantjob.NewErrUnknownOperator(x.Op)
	}
}
