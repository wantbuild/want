package stores

import (
	"context"

	"go.brendoncarroll.net/state/cadata"
)

type Fork struct {
	W cadata.GetPoster
	R cadata.Getter
}

func (s Fork) Post(ctx context.Context, data []byte) (cadata.ID, error) {
	return s.W.Post(ctx, data)
}

func (s Fork) Get(ctx context.Context, id cadata.ID, buf []byte) (int, error) {
	return Union{s.W, s.R}.Get(ctx, id, buf)
}

func (s Fork) Hash(x []byte) cadata.ID {
	return s.W.Hash(x)
}

func (s Fork) MaxSize() int {
	return s.W.MaxSize()
}
