package stores

import (
	"context"
	"slices"

	"blobcache.io/blobcache/src/bcsdk"
	"blobcache.io/blobcache/src/blobcache"
	lru "github.com/hashicorp/golang-lru/v2"
)

type LRUCache struct {
	inner bcsdk.RO
	c     *lru.Cache[blobcache.CID, []byte]
}

func NewLRUCache(inner bcsdk.RO, size int) *LRUCache {
	c, err := lru.New[blobcache.CID, []byte](size)
	if err != nil {
		panic(err)
	}
	return &LRUCache{
		inner: inner,
		c:     c,
	}
}

func (c *LRUCache) Get(ctx context.Context, id blobcache.CID, buf []byte) (int, error) {
	data, ok := c.c.Get(id)
	if ok {
		return copy(buf, data), nil
	}
	n, err := c.inner.Get(ctx, id, buf)
	if err != nil {
		return 0, err
	}
	c.c.Add(id, slices.Clone(buf[:n]))
	return n, nil
}

func (c *LRUCache) Hash(x []byte) blobcache.CID {
	return c.inner.Hash(x)
}

func (c *LRUCache) MaxSize() int {
	return c.inner.MaxSize()
}
