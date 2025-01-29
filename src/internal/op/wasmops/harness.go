//go:build wasm

package wasmops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path"
	"runtime"
	"unsafe"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/stores"
)

//go:wasmimport want input
//go:noescape
func nativeInput(buf unsafe.Pointer, bufLen uint32) int32

//go:wasmimport want output
//go:noescape
func nativeOutput(buf unsafe.Pointer, bufLen uint32) int32

//go:wasmimport want post
//go:noescape
func nativePost(idBuf unsafe.Pointer, buf unsafe.Pointer, bufLen uint32) int32

//go:wasmimport want get
//go:noescape
func nativeGet(buf unsafe.Pointer, bufLen uint32, idBuf unsafe.Pointer) int32

func getInput() ([]byte, error) {
	buf := make([]byte, 1024)
	n := nativeInput(unsafe.Pointer(unsafe.SliceData(buf)), uint32(len(buf)))
	if n < 0 {
		return nil, errors.New("problem reading input")
	}
	return buf[:n], nil
}

func setOutput(buf []byte) error {
	n := nativeOutput(unsafe.Pointer(unsafe.SliceData(buf)), uint32(len(buf)))
	if n != 0 {
		return errors.New("problem setting output")
	}
	return nil
}

type nativeStore struct{}

func (s nativeStore) Post(ctx context.Context, buf []byte) (cadata.ID, error) {
	var id cadata.ID
	ec := nativePost(unsafe.Pointer(&id), unsafe.Pointer(unsafe.SliceData(buf)), uint32(len(buf)))
	if ec != 0 {
		return cadata.ID{}, errors.New("error posting")
	}
	return id, nil
}

func (s nativeStore) Get(ctx context.Context, id cadata.ID, buf []byte) (int, error) {
	n := nativeGet(unsafe.Pointer(unsafe.SliceData(buf)), uint32(len(buf)), unsafe.Pointer(&id))
	if n < 0 {
		return 0, errors.New("error getting")
	}
	if bytes.HasPrefix(buf, id[:]) {
		return 0, cadata.ErrNotFound{Key: id}
	}
	// TODO: remove this
	if s.Hash(buf[:n]) != id {
		panic("bad data from store")
	}
	return int(n), nil
}

func (s nativeStore) Hash(x []byte) cadata.ID {
	return stores.Hash(x)
}

func (s nativeStore) MaxSize() int {
	return stores.MaxBlobSize
}

func Main(fn func(ctx context.Context, s cadata.GetPoster, x glfs.Ref) (*glfs.Ref, error)) {
	inputData, err := getInput()
	if err != nil {
		log.Fatal(err)
	}
	var x glfs.Ref
	if err := json.Unmarshal(inputData, &x); err != nil {
		log.Fatal(err)
	}
	y, err := fn(context.TODO(), nativeStore{}, x)
	if err != nil {
		log.Fatal(err)
	}
	outputData, err := json.Marshal(y)
	if err != nil {
		log.Fatal(err)
	}
	setOutput(outputData)
}

func importPath(ctx context.Context, s cadata.Store, p string, finfo os.FileInfo) (*glfs.Ref, error) {
	if finfo == nil {
		var err error
		finfo, err = os.Stat(p)
		if err != nil {
			return nil, err
		}
	}
	if finfo.IsDir() {
		return importTree(ctx, s, p, finfo)
	}
	if fs.ModeSymlink&finfo.Mode() > 0 {
		return importSymlink(ctx, s, p)
	}
	return importFile(ctx, s, p)
}

func importTree(ctx context.Context, s cadata.Store, p string, finfo os.FileInfo) (*glfs.Ref, error) {
	xs, err := os.ReadDir(p)
	if err != nil {
		return nil, err
	}
	ys := make([]glfs.TreeEntry, len(xs))
	for i := range xs {
		p2 := path.Join(p, xs[i].Name())
		finfo2, err := os.Stat(p2)
		if err != nil {
			return nil, err
		}
		ref, err := importPath(ctx, s, p2, finfo2)
		if err != nil {
			return nil, err
		}
		ys[i] = glfs.TreeEntry{Name: xs[i].Name(), FileMode: finfo.Mode(), Ref: *ref}
	}
	runtime.GC()
	return glfs.PostTreeEntries(ctx, s, ys)
}

func importFile(ctx context.Context, s cadata.Store, p string) (*glfs.Ref, error) {
	f, err := os.OpenFile(p, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return glfs.PostBlob(ctx, s, f)
}

func importSymlink(ctx context.Context, s cadata.Store, p string) (*glfs.Ref, error) {
	return nil, errors.New("symlinks unsupported")
}
