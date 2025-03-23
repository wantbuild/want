package glfsgit

import (
	"context"

	"blobcache.io/glfs"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage"
	"go.brendoncarroll.net/state/cadata"
)

func ImportTree(ctx context.Context, dst cadata.PostExister, gitstor storage.Storer, gt *object.Tree) (*glfs.Ref, error) {
	var ents []glfs.TreeEntry
	for _, ge := range gt.Entries {
		mode, err := ge.Mode.ToOSFileMode()
		if err != nil {
			return nil, err
		}
		var ref *glfs.Ref
		if ge.Mode.IsFile() {
			eo, err := gitstor.EncodedObject(plumbing.BlobObject, ge.Hash)
			if err != nil {
				return nil, err
			}
			gb, err := object.DecodeBlob(eo)
			if err != nil {
				return nil, err
			}
			if ref, err = ImportBlob(ctx, dst, gitstor, gb); err != nil {
				return nil, err
			}
		} else {
			eo, err := gitstor.EncodedObject(plumbing.TreeObject, ge.Hash)
			if err != nil {
				return nil, err
			}
			gt, err := object.DecodeTree(gitstor, eo)
			if err != nil {
				return nil, err
			}
			if ref, err = ImportTree(ctx, dst, gitstor, gt); err != nil {
				return nil, err
			}
		}
		ents = append(ents, glfs.TreeEntry{
			Name:     ge.Name,
			FileMode: mode,
			Ref:      *ref,
		})
	}
	return glfs.PostTreeSlice(ctx, dst, ents)
}

func ImportBlob(ctx context.Context, dst cadata.PostExister, gitstore storage.Storer, gb *object.Blob) (*glfs.Ref, error) {
	rc, err := gb.Reader()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return glfs.PostBlob(ctx, dst, rc)
}
