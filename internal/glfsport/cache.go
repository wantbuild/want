package glfsport

import (
	"context"
	"io/fs"
	"os"
	"sync"
	"time"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state"
	"go.brendoncarroll.net/state/kv"
)

type Entry struct {
	Size    int64       `json:"size"`
	Mode    os.FileMode `json:"mode"`
	ModTime time.Time   `json:"modTime"`
	Ref     glfs.Ref    `json:"ref"`
}

func (e *Entry) Matches(finfo fs.FileInfo) bool {
	return e.ModTime.Equal(finfo.ModTime()) &&
		e.Mode == finfo.Mode() &&
		e.Size == finfo.Size()
}

type Cache interface {
	kv.Getter[string, Entry]
	kv.Putter[string, Entry]
}

var _ Cache = NullCache{}

type NullCache struct{}

func (c NullCache) Get(ctx context.Context, p string, dst *Entry) error {
	return state.ErrNotFound[string]{Key: p}
}

func (c NullCache) Put(ctx context.Context, p string, e Entry) error {
	return nil
}

var _ Cache = &MemCache{}

type MemCache struct {
	mu sync.RWMutex
	m  map[string]Entry
}

func (mc *MemCache) Get(ctx context.Context, p string, dst *Entry) error {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	ent, exists := mc.m[p]
	if !exists {
		return state.ErrNotFound[string]{Key: p}
	}
	*dst = ent
	return nil
}

func (mc *MemCache) Put(ctx context.Context, p string, e Entry) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	if mc.m == nil {
		mc.m = make(map[string]Entry)
	}
	mc.m[p] = e
	return nil
}
