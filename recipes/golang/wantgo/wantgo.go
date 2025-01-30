package wantgo

import (
	"context"
	"path"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/wantjob"
)

const OpMakeTestExec = wantjob.OpName("golang.makeTestExec")

// IsGoModule returns (true, nil) if the directory is a Go Module
func IsGoModule(ctx context.Context, s cadata.Getter, x glfs.Ref) (bool, error) {
	tree, err := glfs.GetTree(ctx, s, x)
	if err != nil {
		return false, err
	}
	if ent := tree.Lookup("go.mod"); ent == nil {
		return false, nil
	}
	if ent := tree.Lookup("go.sum"); ent == nil {
		return false, nil
	}
	return true, nil
}

// IsGoPackage returns (true, nil) if the directory at x is a Go Package
// Go packages contain 1 or more .go files.
// It will return (false, nil) if the package is not a Go package
func IsGoPackage(ctx context.Context, s cadata.Getter, x glfs.Ref) (bool, error) {
	const goExt = ".go"
	if x.Type != glfs.TypeTree {
		return false, nil
	}
	tree, err := glfs.GetTree(ctx, s, x)
	if err != nil {
		return false, err
	}
	for _, ent := range tree.Entries {
		if path.Ext(ent.Name) == goExt {
			return true, nil
		}
	}
	return false, nil
}
