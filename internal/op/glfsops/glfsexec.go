package glfsops

import (
	"context"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/stores"
	"wantbuild.io/want/internal/wantjob"
)

var _ wantjob.Executor = Executor{}

type Executor struct {
	s cadata.Store
}

func NewExecutor(s cadata.Store) Executor {
	return Executor{s: s}
}

func (e Executor) Compute(ctx context.Context, jc *wantjob.Ctx, src cadata.Getter, x wantjob.Task) (*glfs.Ref, error) {
	op, ok := ops[x.Op]
	if !ok {
		return nil, wantjob.NewErrUnknownOperator(x.Op)
	}
	return op(ctx, stores.Fork{W: e.s, R: src}, x.Input)
}

func (e Executor) GetStore() cadata.Getter {
	return e.s
}
