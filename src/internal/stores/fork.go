package stores

import (
	"context"

	"blobcache.io/blobcache/src/bcsdk"
	"blobcache.io/blobcache/src/blobcache"
)

type Fork struct {
	W bcsdk.WO
	R bcsdk.RO
}

func (s Fork) Post(ctx context.Context, data []byte) (blobcache.CID, error) {
	return s.W.Post(ctx, data)
}

func (s Fork) Exists(ctx context.Context, cids []blobcache.CID, dst *blobcache.BitMap) error {
	if err := s.W.Exists(ctx, cids, dst); err != nil {
		return err
	}
	if err := s.R.Exists(ctx, cids, dst); err != nil {
		return err
	}
	return nil
}

func (s Fork) Get(ctx context.Context, id blobcache.CID, buf []byte) (int, error) {
	return s.R.Get(ctx, id, buf)
}

func (s Fork) Hash(x []byte) blobcache.CID {
	return s.W.Hash(x)
}

func (s Fork) MaxSize() int {
	return s.W.MaxSize()
}
