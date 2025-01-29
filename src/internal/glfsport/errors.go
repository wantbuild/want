package glfsport

import (
	"fmt"
	"io/fs"
)

// ErrUnexpectedFile is returned by Export when it encounters a file that it has never seen
// and should not overwrite.
type ErrUnexpectedFile struct {
	Path string
	Info fs.FileInfo
}

func (e ErrUnexpectedFile) Error() string {
	return fmt.Sprintf("unexpected file %s %v", e.Path, e.Info.Mode())
}

// ErrStaleStale is returned by Export when the cache entry for a file does not match what is in the cache.
type ErrStaleCache struct {
	Path       string
	Info       fs.FileInfo
	CacheEntry Entry
}

func (e ErrStaleCache) Error() string {
	return fmt.Sprintf("modified at has changed for file %s %v != %v", e.Path, e.Info, e.CacheEntry)
}
