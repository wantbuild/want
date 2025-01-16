package glfsops

import (
	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/glfstasks"
	"wantbuild.io/want/internal/stores"
	"wantbuild.io/want/lib/wantjob"
)

var _ wantjob.Executor = Executor{}

type Executor struct {
	s cadata.GetPoster
}

func NewExecutor(s cadata.GetPoster) Executor {
	return Executor{s: s}
}

func (e Executor) Execute(jc *wantjob.Ctx, src cadata.Getter, x wantjob.Task) ([]byte, error) {
	op, ok := ops[x.Op]
	if !ok {
		return nil, wantjob.NewErrUnknownOperator(x.Op)
	}
	return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
		return op(jc.Context(), stores.Fork{W: e.s, R: src}, x)
	})
}

func (e Executor) GetStore() cadata.Getter {
	return e.s
}
