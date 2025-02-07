package wantc

import (
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/google/go-jsonnet"
	"go.brendoncarroll.net/state/cadata"
)

var _ jsonnet.Importer = &jsImporter{}

// jsImporter is a jsonnet.Importer which imports from a build's VFS
type jsImporter struct {
	resolve func(ModuleID, string) (ModuleID, error)
	load    func(FQPath) ([]byte, error)

	mu    sync.RWMutex
	cache map[FQPath]jsonnet.Contents
}

func newImporter(resolve func(ModuleID, string) (ModuleID, error), load func(fqp FQPath) ([]byte, error)) *jsImporter {
	return &jsImporter{
		resolve: resolve,
		load:    load,

		cache: make(map[FQPath]jsonnet.Contents),
	}
}

func (imp *jsImporter) Import(importedFrom, importedPath string) (contents jsonnet.Contents, foundAt string, err error) {
	var fromFQP, targetFQP FQPath
	if importedFrom == "" {
		// importedFrom is the empty string when first importing the root file.
		fqp, err := parseJsonnetPath(importedPath)
		if err != nil {
			return jsonnet.Contents{}, "", err
		}
		targetFQP = *fqp
	} else {
		fromFQP_, err := parseJsonnetPath(importedFrom)
		if err != nil {
			return jsonnet.Contents{}, "", err
		}
		fromFQP = *fromFQP_
		targetFQP = FQPath{ModuleID: fromFQP.ModuleID, Path: importedPath}
	}

	if after, yes := strings.CutPrefix(targetFQP.Path, "@"); yes {
		parts := strings.SplitN(after, "/", 2)
		modid, err := imp.resolve(targetFQP.ModuleID, parts[0])
		if err != nil {
			return jsonnet.Contents{}, "", err
		}
		targetFQP.ModuleID = modid
		if len(parts) > 1 {
			targetFQP.Path = parts[1]
		}
	} else {
		targetFQP.Path = PathFrom(fromFQP.Path, targetFQP.Path)
	}

	imp.mu.RLock()
	if contents, exists := imp.cache[targetFQP]; exists {
		imp.mu.RUnlock()
		return contents, mkJsonnetPath(targetFQP), nil
	}
	imp.mu.RUnlock()

	data, err := imp.load(targetFQP)
	if err != nil {
		return jsonnet.Contents{}, "", fmt.Errorf("importing %q from %q: %w", importedPath, importedFrom, err)
	}
	imp.mu.Lock()
	defer imp.mu.Unlock()
	if c, exists := imp.cache[targetFQP]; exists {
		return c, mkJsonnetPath(targetFQP), nil
	} else {
		switch path.Ext(importedPath) {
		case ".libsonnet", ".want", ".wants":
			data = slices.Concat(
				[]byte(LocalGround(targetFQP)),
				[]byte(LocalDerived(targetFQP)),
				data,
			)
		}
		c := jsonnet.MakeContentsRaw(data)
		imp.cache[targetFQP] = c
		return c, mkJsonnetPath(targetFQP), nil
	}
}

func newJsonnetVM(imp jsonnet.Importer, md map[string]any) *jsonnet.VM {
	vm := jsonnet.MakeVM()
	vm.Importer(imp)
	data, err := json.Marshal(md)
	if err != nil {
		panic(err)
	}
	vm.ExtCode("metadata", string(data))
	return vm
}

func LocalGround(fqp FQPath) string {
	return fmt.Sprintf(`local GROUND = {"module":"%v","derived":false,"callerPath":"%s"};`, fqp.ModuleID.String(), fqp.Path)
}

func LocalDerived(fqp FQPath) string {
	return fmt.Sprintf(`local DERIVED = {"module":"%v","derived":true,"callerPath":"%s"};`, fqp.ModuleID.String(), fqp.Path)
}

func mkJsonnetPath(fqp FQPath) string {
	return fqp.ModuleID.String() + ":" + fqp.Path
}

func parseJsonnetPath(x string) (*FQPath, error) {
	parts := strings.SplitN(x, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("could not parse %q as a fq path", x)
	}
	var modCID cadata.ID
	if err := modCID.UnmarshalBase64([]byte(parts[0])); err != nil {
		return nil, nil
	}
	return &FQPath{
		ModuleID: modCID,
		Path:     parts[1],
	}, nil
}
