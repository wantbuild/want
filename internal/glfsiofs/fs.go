package glfsiofs

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"
)

var _ fs.FS = &GLFSFS{}

type GLFSFS struct {
	ag   *glfs.Agent
	s    cadata.Getter
	root glfs.Ref
}

func New(s cadata.Getter, root glfs.Ref) *GLFSFS {
	return &GLFSFS{
		ag:   glfs.NewAgent(),
		s:    s,
		root: root,
	}
}

func (s *GLFSFS) Open(p string) (fs.File, error) {
	ctx := context.TODO()
	p = strings.Trim(p, "/")
	if p == "." {
		p = ""
	}
	ref, err := s.ag.GetAtPath(ctx, s.s, s.root, p)
	if err != nil {
		return nil, err
	}
	return newGLFSFile(ctx, s.ag, s.s, p, *ref), nil
}

var _ fs.File = &GLFSFile{}

type GLFSFile struct {
	ctx  context.Context
	ag   *glfs.Agent
	s    cadata.Getter
	ref  glfs.Ref
	name string
	mode os.FileMode
	r    *glfs.Reader
}

func newGLFSFile(ctx context.Context, ag *glfs.Agent, s cadata.Getter, name string, ref glfs.Ref) *GLFSFile {
	return &GLFSFile{
		ctx:  ctx,
		ag:   ag,
		s:    s,
		name: name,
		ref:  ref,
	}
}

func (f *GLFSFile) Read(buf []byte) (int, error) {
	if f.ref.Type == glfs.TypeTree {
		return 0, fs.ErrInvalid
	}
	ctx := f.ctx
	if f.r == nil {
		r, err := f.ag.GetBlob(ctx, f.s, f.ref)
		if err != nil {
			return 0, err
		}
		f.r = r
	}
	n, err := f.r.Read(buf)
	return n, err
}

func (f *GLFSFile) Stat() (fs.FileInfo, error) {
	if f.ref.Type == glfs.TypeTree {
		return &fileInfo{name: f.name, mode: fs.ModeDir}, nil
	}
	return &fileInfo{
		mode: f.mode,
		size: int64(f.ref.Size),
	}, nil
}

func (f *GLFSFile) Close() error {
	return nil
}

func (f *GLFSFile) ReadDir(n int) (ret []fs.DirEntry, _ error) {
	ctx := context.TODO()
	if f.ref.Type != glfs.TypeTree {
		return nil, errors.New("read on non-Tree")
	}
	t, err := f.ag.GetTree(ctx, f.s, f.ref)
	if err != nil {
		return nil, err
	}
	for _, ent := range t.Entries {
		ret = append(ret, dirEnt{
			name: ent.Name,
			mode: ent.FileMode,
		})
	}
	return ret, nil
}

type fileInfo struct {
	name    string
	mode    fs.FileMode
	modTime time.Time
	size    int64
}

func (fi fileInfo) Name() string {
	return fi.name
}

func (fi fileInfo) Mode() fs.FileMode {
	return fi.mode
}

func (fi fileInfo) ModTime() time.Time {
	return fi.modTime
}

func (fi fileInfo) IsDir() bool {
	return fi.mode.IsDir()
}

func (fi fileInfo) Sys() any {
	return nil
}

func (fi fileInfo) Size() int64 {
	return fi.size
}

type dirEnt struct {
	name string
	mode fs.FileMode
}

func (de dirEnt) Name() string {
	return de.name
}

func (de dirEnt) Info() (fs.FileInfo, error) {
	return nil, nil
}

func (dr dirEnt) IsDir() bool {
	return dr.mode.IsDir()
}

func (dr dirEnt) Type() fs.FileMode {
	return dr.mode & fs.ModeType
}
