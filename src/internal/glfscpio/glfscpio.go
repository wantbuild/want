package glfscpio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"

	"github.com/blobcache/glfs"
	"github.com/u-root/u-root/pkg/cpio"
	"go.brendoncarroll.net/state/cadata"
)

const MaxPathLen = 4096

// Write serializes the filesystem at x into a CPIO archive and writes it to w.
func Write(ctx context.Context, s cadata.Getter, root glfs.Ref, w io.Writer) error {
	if root.Type != glfs.TypeTree {
		return fmt.Errorf("cannot write non-tree as CPIO archive")
	}
	recWriter := cpio.Newc.Writer(w)
	if err := glfs.WalkTree(ctx, s, root, func(prefix string, ent glfs.TreeEntry) error {
		p := path.Join(prefix, ent.Name)
		var rec cpio.Record
		switch ent.Ref.Type {
		case glfs.TypeTree:
			rec = cpio.Directory(p, uint64(ent.FileMode))
		default:
			switch {
			case ent.FileMode&fs.ModeSymlink > 0:
				targetPath, err := glfs.GetBlobBytes(ctx, s, ent.Ref, MaxPathLen)
				if err != nil {
					return err
				}
				rec = cpio.Symlink(p, string(targetPath))
			default:
				r, err := glfs.GetBlob(ctx, s, ent.Ref)
				if err != nil {
					return err
				}
				rec = cpio.Record{
					ReaderAt: r,
					Info: cpio.Info{
						Name:     p,
						FileSize: ent.Ref.Size,
						Mode:     uint64(ent.FileMode),
					},
				}
			}
		}
		return recWriter.WriteRecord(rec)
	}); err != nil {
		return err
	}
	return nil
}

func Read(ctx context.Context, s cadata.PostExister, r io.ReaderAt) (*glfs.Ref, error) {
	recReader := cpio.Newc.Reader(r)

	var ents []glfs.TreeEntry
	for {
		rec, err := recReader.ReadRecord()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		mode := fs.FileMode(rec.Mode)
		if mode.IsDir() {
		} else {
			sr := io.NewSectionReader(rec.ReaderAt, 0, int64(rec.FileSize))
			ref, err := glfs.PostBlob(ctx, s, sr)
			if err != nil {
				return nil, err
			}
			ents = append(ents, glfs.TreeEntry{
				Name:     rec.Name,
				FileMode: mode,
				Ref:      *ref,
			})
		}
	}
	return glfs.PostTreeSlice(ctx, s, ents)
}
