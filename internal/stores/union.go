package stores

import (
	"context"

	"go.brendoncarroll.net/state/cadata"
)

type Union []cadata.Getter

func (u Union) Get(ctx context.Context, id cadata.ID, buf []byte) (int, error) {
	var err error
	for _, s := range u {
		var n int
		n, err = s.Get(ctx, id, buf)
		if err == nil {
			return n, nil
		}
	}
	if err == nil {
		err = cadata.ErrNotFound{Key: id}
	}
	return 0, err
}

func (u Union) Hash(x []byte) cadata.ID {
	return u[0].Hash(x)
}

func (u Union) MaxSize() int {
	return u[0].MaxSize()
}
