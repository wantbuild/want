package stores

import (
	"context"

	"blobcache.io/blobcache/src/bcsdk"
	"blobcache.io/blobcache/src/blobcache"
)

type Union []bcsdk.RO

func (u Union) Get(ctx context.Context, id blobcache.CID, buf []byte) (int, error) {
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
		err = blobcache.ErrNotFound{CID: id}
	}
	return 0, err
}

func (u Union) Exists(ctx context.Context, cids []blobcache.CID, dst []bool) error {
	for _, s := range u {
		if err := s.Exists(ctx, cids, dst); err != nil {
			return err
		}
	}
	return nil
}

func (u Union) Hash(x []byte) blobcache.CID {
	return Hash(x)
}

func (u Union) MaxSize() int {
	return MaxBlobSize
}
