package wantc

import (
	"context"
	"encoding/json"
	"path"

	"github.com/blobcache/glfs"
	"github.com/google/go-jsonnet"
	"go.brendoncarroll.net/state/cadata"
	"wantbuild.io/want/src/wantcfg"
)

type ModuleID = cadata.ID

func NewModuleID(x glfs.Ref) ModuleID {
	// TODO: hash DEK and CID together?
	return x.CID
}

type FQPath struct {
	ModuleID ModuleID
	Path     string
}

type Namespace map[string]glfs.Ref

// IsModule returns true if x is valid Want Module.
func IsModule(ctx context.Context, src cadata.Getter, x glfs.Ref) (bool, error) {
	if x.Type != glfs.TypeTree {
		return false, nil
	}
	t, err := glfs.GetTreeSlice(ctx, src, x, 1e6)
	if err != nil {
		return false, err
	}
	ent := glfs.Lookup(t, WantFilename)
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

// GetModuleConfig gets the WANT file from the directory at modRef, evaluates it and returns it.
func GetModuleConfig(ctx context.Context, src cadata.Getter, modRef glfs.Ref) (*wantcfg.ModuleConfig, error) {
	cfgRef, err := glfs.GetAtPath(ctx, src, modRef, "WANT")
	if err != nil {
		return nil, err
	}
	data, err := glfs.GetBlobBytes(ctx, src, *cfgRef, MaxJsonnetFileSize)
	if err != nil {
		return nil, err
	}
	return ParseModuleConfig(data)
}

func ParseModuleConfig(x []byte) (*wantcfg.ModuleConfig, error) {
	vm := jsonnet.MakeVM()
	vm.Importer(&snippetImporter{})
	jsonData, err := vm.EvaluateAnonymousSnippet("WANT", string(x))
	if err != nil {
		return nil, err
	}
	var ret wantcfg.ModuleConfig
	if err := json.Unmarshal([]byte(jsonData), &ret); err != nil {
		return nil, err
	}
	return &ret, nil
}
