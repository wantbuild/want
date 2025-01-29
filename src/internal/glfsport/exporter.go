package glfsport

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state"
	"go.brendoncarroll.net/state/cadata"
	"go.brendoncarroll.net/state/kv"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

const MaxPathLen = 4096

type Exporter struct {
	Store  cadata.Getter
	Dir    string
	Cache  Cache
	Filter func(p string) bool
}

func (ex Exporter) Export(ctx context.Context, root glfs.Ref, target string) error {
	sem := semaphore.NewWeighted(int64(2 * runtime.GOMAXPROCS(0)))
	switch root.Type {
	case glfs.TypeTree:
		return ex.exportTree(ctx, sem, root, 0o755, target)
	case glfs.TypeBlob:
		return ex.exportFile(ctx, root, 0o644, target)
	default:
		return fmt.Errorf("cannot export type: %v", root.Type)
	}
}

func (ex Exporter) exportTree(ctx context.Context, sem *semaphore.Weighted, root glfs.Ref, mode os.FileMode, p string) error {
	finfo, err := ex.stat(p)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && !finfo.IsDir() {
		return fmt.Errorf("cannot export tree %v to path %s", root, p)
	}
	tree, err := glfs.GetTree(ctx, ex.Store, root)
	if err != nil {
		return err
	}
	// create with 0o755 so we have permission to modify it.
	if err := ex.mkdir(p, 0o755); err != nil && !os.IsExist(err) {
		return err
	}
	eg, ctx := errgroup.WithContext(ctx)
	for _, ent := range tree.Entries {
		ent := ent
		fn := func() error {
			p2 := path.Join(p, ent.Name)
			switch {
			case ent.Ref.Type == glfs.TypeTree:
				return ex.exportTree(ctx, sem, ent.Ref, ent.FileMode, p2)
			case ent.FileMode.IsRegular():
				return ex.exportFile(ctx, ent.Ref, ent.FileMode, p2)
			case ent.FileMode&fs.ModeSymlink > 0:
				return ex.exportLink(ctx, ent.Ref, ent.FileMode, p2)
			default:
				return fmt.Errorf("cannot export entry with mode %v", ent.FileMode)
			}
		}
		if sem.TryAcquire(1) {
			eg.Go(fn)
		} else {
			if err := fn(); err != nil {
				return err
			}
		}
	}
	if err := eg.Wait(); err != nil {
		return err
	}
	return ex.chmod(p, mode.Perm())
}

func (ex Exporter) exportLink(ctx context.Context, ref glfs.Ref, mode os.FileMode, p string) error {
	pathData, err := glfs.GetBlobBytes(ctx, ex.Store, ref, MaxPathLen)
	if err != nil {
		return err
	}
	return ex.symlink(string(pathData), p)
}

func (ex Exporter) exportFile(ctx context.Context, ref glfs.Ref, mode os.FileMode, p string) error {
	finfo, err := ex.stat(p)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && finfo.Mode().IsDir() {
		return fmt.Errorf("cannot export. dir at path %v", p)
	}
	if err == nil && !finfo.Mode().IsDir() {
		// lookup last export
		ent, err := kv.Get(ctx, ex.Cache, p)
		if err != nil && !state.IsErrNotFound[string](err) {
			return err
		}
		if state.IsErrNotFound[string](err) {
			return ErrUnexpectedFile{
				Path: p,
				Info: finfo,
			}
		}
		if !ent.Matches(finfo) {
			return ErrStaleCache{
				Path:       p,
				Info:       finfo,
				CacheEntry: ent,
			}
		}
	}
	// at this point:
	// - there isn't a non-file at p
	// - if there was a file, we exported it and it hasn't been modified
	// - otherwise there is no file
	r, err := glfs.GetBlob(ctx, ex.Store, ref)
	if err != nil {
		return err
	}
	if err := ex.putFile(ctx, p, mode, r); err != nil {
		return err
	}
	finfo, err = ex.stat(p)
	if err != nil {
		return err
	}
	ent := Entry{
		Size:    finfo.Size(),
		Mode:    finfo.Mode(),
		Ref:     ref,
		ModTime: finfo.ModTime(),
	}
	return ex.Cache.Put(ctx, p, ent)
}

func (ex *Exporter) stat(p string) (fs.FileInfo, error) {
	p2, err := ex.path(p)
	if err != nil {
		return nil, err
	}
	return os.Stat(p2)
}

func (ex *Exporter) putFile(ctx context.Context, p string, mode os.FileMode, r io.Reader) error {
	p2, err := ex.path(p)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(p2, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return err
	}
	return f.Close()
}

func (ex *Exporter) symlink(target, newp string) error {
	p2, err := ex.path(newp)
	if err != nil {
		return err
	}
	return os.Symlink(target, p2)
}

func (ex *Exporter) mkdir(p string, mode os.FileMode) error {
	p2, err := ex.path(p)
	if err != nil {
		return err
	}
	return os.Mkdir(p2, mode)
}

func (ex *Exporter) chmod(p string, mode fs.FileMode) error {
	p2, err := ex.path(p)
	if err != nil {
		return err
	}
	return os.Chmod(p2, mode)
}

func (ex *Exporter) path(x string) (string, error) {
	if ex.Dir == "" {
		panic("Exporter must have Dir set")
	}
	x = glfs.CleanPath(x)
	if ex.Filter != nil && !ex.Filter(x) {
		return "", fmt.Errorf("cannot export to filtered path %q", x)
	}
	return filepath.Join(ex.Dir, filepath.FromSlash(x)), nil
}
