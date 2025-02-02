package wantgo

import (
	"context"
	"path"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/exp/streams"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/wantjob"
)

const OpMakeTestExec = wantjob.OpName("golang.makeTestExec")

// IsGoModule returns (true, nil) if the directory is a Go Module
func IsGoModule(ctx context.Context, s cadata.Getter, x glfs.Ref) (bool, error) {
	tr, err := glfs.NewAgent().NewTreeReader(s, x)
	if err != nil {
		return false, err
	}
	var foundMod, foundSum bool
	for {
		ent, err := streams.Next(ctx, tr)
		if err != nil {
			if streams.IsEOS(err) {
				break
			}
			return false, err
		}
		switch ent.Name {
		case "go.mod":
			foundMod = true
		case "go.sum":
			foundSum = true
		}
		if foundMod && foundSum {
			return true, nil
		}
	}
	return false, nil
}

// IsGoPackage returns (true, nil) if the directory at x is a Go Package
// Go packages contain 1 or more .go files.
// It will return (false, nil) if the package is not a Go package
func IsGoPackage(ctx context.Context, s cadata.Getter, x glfs.Ref) (bool, error) {
	const goExt = ".go"
	if x.Type != glfs.TypeTree {
		return false, nil
	}
	tr, err := glfs.NewAgent().NewTreeReader(s, x)
	if err != nil {
		return false, err
	}
	for {
		ent, err := streams.Next(ctx, tr)
		if err != nil {
			if streams.IsEOS(err) {
				break
			}
			return false, err
		}
		if path.Ext(ent.Name) == goExt {
			return true, nil
		}
	}
	return false, nil
}
