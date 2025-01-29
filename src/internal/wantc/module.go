package wantc

import (
	"context"
	"path"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"
)

// IsModule returns true if x is valid Want Module.
func IsModule(ctx context.Context, src cadata.Getter, x glfs.Ref) (bool, error) {
	if x.Type != glfs.TypeTree {
		return false, nil
	}
	t, err := glfs.GetTree(ctx, src, x)
	if err != nil {
		return false, err
	}
	ent := t.Lookup(WantFilename)
	if ent == nil {
		return false, nil
	}
	return true, nil
}

// FindModules finds all the modules in root.
// Usually root itself is a module, and there could be submodules within root.
func FindModules(ctx context.Context, src cadata.Getter, root glfs.Ref) (map[string]glfs.Ref, error) {
	modules := make(map[string]glfs.Ref)
	if err := glfs.WalkTree(ctx, src, root, func(prefix string, ent glfs.TreeEntry) error {
		p := path.Join(prefix, ent.Name)
		yes, err := IsModule(ctx, src, ent.Ref)
		if err != nil {
			return err
		}
		if yes {
			modules[p] = ent.Ref
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return modules, nil
}
