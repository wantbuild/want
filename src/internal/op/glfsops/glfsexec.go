package glfsops

import (
	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/wantjob"
)

var _ wantjob.Executor = Executor{}

type Executor struct{}

func (e Executor) Execute(jc wantjob.Ctx, src cadata.Getter, x wantjob.Task) ([]byte, error) {
	op, ok := ops[x.Op]
	if !ok {
		return nil, wantjob.NewErrUnknownOperator(x.Op)
	}
	return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
		return op(jc.Context, stores.Fork{W: jc.Dst, R: src}, x)
	})
}
