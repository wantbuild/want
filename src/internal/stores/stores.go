package stores

import (
	"context"

	"go.brendoncarroll.net/state/cadata"
	"lukechampine.com/blake3"
)

const MaxBlobSize = 1 << 21

func Hash(x []byte) cadata.ID {
	return blake3.Sum256(x)
}

func NewMem() cadata.Store {
	return cadata.NewMem(Hash, MaxBlobSize)
}

func NewVoid() cadata.Getter {
	return Union{}
}

func ExistsOnGet(ctx context.Context, src cadata.Getter, id cadata.ID) (bool, error) {
	if e, ok := src.(cadata.Exister); ok {
		return e.Exists(ctx, id)
	}
	return ExistsUsingGet(ctx, src, id)
}

func ExistsUsingGet(ctx context.Context, src cadata.Getter, id cadata.ID) (bool, error) {
	_, err := src.Get(ctx, id, make([]byte, src.MaxSize()))
	if err != nil {
		if cadata.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
