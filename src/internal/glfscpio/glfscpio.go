package glfscpio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"

	"github.com/blobcache/glfs"
	"github.com/cavaliergopher/cpio"
	"go.brendoncarroll.net/state/cadata"
)

const MaxPathLen = 4096

// Write serializes the filesystem at x into a CPIO archive and writes it to w.
func Write(ctx context.Context, s cadata.Getter, root glfs.Ref, w io.Writer) error {
	if root.Type != glfs.TypeTree {
		return fmt.Errorf("cannot write non-tree as CPIO archive")
	}
	cpw := cpio.NewWriter(w)
	// root record
	if err := cpw.WriteHeader(&cpio.Header{
		Name:  ".",
		Links: 1,
		Mode:  cpio.FileMode(1<<14 | 0o755),
	}); err != nil {
		return err
	}
	if err := glfs.WalkTree(ctx, s, root, func(prefix string, ent glfs.TreeEntry) error {
		p := path.Join(prefix, ent.Name)
		mode := cpio.FileMode(ent.FileMode)
		switch {
		case ent.Ref.Type == glfs.TypeTree:
			if err := cpw.WriteHeader(&cpio.Header{
				Name:  p,
				Mode:  mode | cpio.FileMode(fs.ModeDir),
				Links: 1,
			}); err != nil {
				return err
			}
		case ent.Ref.Type == glfs.TypeBlob && ent.FileMode&fs.ModeSymlink > 0:
			data, err := glfs.GetBlobBytes(ctx, s, ent.Ref, MaxPathLen)
			if err != nil {
				return err
			}
			if err := cpw.WriteHeader(&cpio.Header{
				Name:     p,
				Linkname: string(data),
				Size:     int64(ent.Ref.Size),
				Mode:     mode,
				Links:    1,
			}); err != nil {
				return err
			}
			if _, err := cpw.Write(data); err != nil {
				return err
			}
		default:
			if err := cpw.WriteHeader(&cpio.Header{
				Name:  p,
				Size:  int64(ent.Ref.Size),
				Mode:  mode,
				Links: 1,
			}); err != nil {
				return err
			}
			r, err := glfs.GetBlob(ctx, s, ent.Ref)
			if err != nil {
				return err
			}
			if n, err := io.Copy(cpw, r); err != nil {
				return err
			} else if n != int64(ent.Ref.Size) {
				return fmt.Errorf("io.Copy copied wrong number of bytes")
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return cpw.Close()
}

func Read(ctx context.Context, s cadata.PostExister, r io.Reader) (*glfs.Ref, error) {
	cpr := cpio.NewReader(r)
	var ents []glfs.TreeEntry
	for {
		h, err := cpr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		if h.Name == "." || h.Name == "" {
			continue
		}
		mode := fs.FileMode(h.Mode)
		if mode.IsDir() {
		} else {
			ref, err := glfs.PostBlob(ctx, s, cpr)
			if err != nil {
				return nil, err
			}
			ents = append(ents, glfs.TreeEntry{
				Name:     h.Name,
				FileMode: mode & fs.ModePerm,
				Ref:      *ref,
			})
		}
	}
	return glfs.PostTreeSlice(ctx, s, ents)
}
