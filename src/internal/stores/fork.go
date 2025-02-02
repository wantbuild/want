package stores

import (
	"context"

	"go.brendoncarroll.net/state/cadata"
)

type Fork struct {
	W cadata.PostExister
	R cadata.Getter
}

func (s Fork) Post(ctx context.Context, data []byte) (cadata.ID, error) {
	return s.W.Post(ctx, data)
}

func (s Fork) Exists(ctx context.Context, id cadata.ID) (bool, error) {
	if yes, err := s.W.Exists(ctx, id); err == nil && yes {
		return yes, nil
	}
	return ExistsOnGet(ctx, s.R, id)
}

func (s Fork) Get(ctx context.Context, id cadata.ID, buf []byte) (int, error) {
	return s.R.Get(ctx, id, buf)
}

func (s Fork) Hash(x []byte) cadata.ID {
	return s.W.Hash(x)
}

func (s Fork) MaxSize() int {
	return s.W.MaxSize()
}
