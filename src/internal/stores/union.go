package stores

import (
	"context"

	"go.brendoncarroll.net/state/cadata"
)

type Union []cadata.Getter

func (u Union) Get(ctx context.Context, id cadata.ID, buf []byte) (int, error) {
	var err error
	for i := len(u) - 1; i >= 0; i-- {
		s := u[i]
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
	return Hash(x)
}

func (u Union) MaxSize() int {
	return MaxBlobSize
}
