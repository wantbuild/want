package stores

import (
	"context"
	"slices"

	lru "github.com/hashicorp/golang-lru/v2"
	"go.brendoncarroll.net/state/cadata"
)

type LRUCache struct {
	inner cadata.Getter
	c     *lru.Cache[cadata.ID, []byte]
}

func NewLRUCache(inner cadata.Getter, size int) *LRUCache {
	c, err := lru.New[cadata.ID, []byte](size)
	if err != nil {
		panic(err)
	}
	return &LRUCache{
		inner: inner,
		c:     c,
	}
}

func (c *LRUCache) Get(ctx context.Context, id cadata.ID, buf []byte) (int, error) {
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

func (c *LRUCache) Hash(x []byte) cadata.ID {
	return c.inner.Hash(x)
}

func (c *LRUCache) MaxSize() int {
	return c.inner.MaxSize()
}
