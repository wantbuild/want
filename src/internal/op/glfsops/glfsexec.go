package glfsops

import (
	"fmt"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/wantjob"
)

var _ wantjob.Executor = Executor{}

type Executor struct{}

func (e Executor) Execute(jc wantjob.Ctx, src cadata.Getter, x wantjob.Task) wantjob.Result {
	op, ok := ops[x.Op]
	if !ok {
		return *wantjob.Result_ErrExec(wantjob.NewErrUnknownOperator(x.Op))
	}
	return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
		ref, err := op(jc.Context, jc.Dst, src, x)
		if ref != nil {
			if err := glfstasks.FastSync(jc.Context, jc.Dst, src, *ref); err != nil {
				return nil, fmt.Errorf("glfsops: syncing %w", err)
			}
		}
		return ref, err
	})
}
