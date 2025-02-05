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
	"wantbuild.io/want/src/wantcfg"
)

var _ jsonnet.Importer = &jsImporter{}

// jsImporter is a jsonnet.Importer which imports from a build's VFS
type jsImporter struct {
	load    func(FQPath) ([]byte, error)
	libWant jsonnet.Contents

	mu    sync.RWMutex
	cache map[FQPath]jsonnet.Contents
}

func newImporter(load func(fqp FQPath) ([]byte, error)) *jsImporter {
	return &jsImporter{
		libWant: jsonnet.MakeContents(wantcfg.LibWant()),
		load:    load,
		cache:   make(map[FQPath]jsonnet.Contents),
	}
}

func (imp *jsImporter) Import(importedFrom, importedPath string) (contents jsonnet.Contents, foundAt string, err error) {
	if importedPath == "want" {
		return imp.libWant, "want", nil
	}
	var fqp *FQPath
	if importedFrom == "" {
		// this is the root import for the VM to load the file.
		var err error
		fqp, err = parseJsonnetPath(importedPath)
		if err != nil {
			return jsonnet.Contents{}, "", err
		}
	} else {
		fqp, err = parseJsonnetPath(importedFrom)
		if err != nil {
			return jsonnet.Contents{}, "", err
		}
		fqp.Path = PathFrom(fqp.Path, importedPath)
	}

	imp.mu.RLock()
	if contents, exists := imp.cache[*fqp]; exists {
		imp.mu.RUnlock()
		return contents, mkJsonnetPath(*fqp), nil
	}
	imp.mu.RUnlock()

	data, err := imp.load(*fqp)
	if err != nil {
		return jsonnet.Contents{}, "", fmt.Errorf("importing %q from %q: %w", importedPath, importedFrom, err)
	}
	imp.mu.Lock()
	defer imp.mu.Unlock()
	if c, exists := imp.cache[*fqp]; exists {
		return c, mkJsonnetPath(*fqp), nil
	} else {
		switch path.Ext(importedPath) {
		case ".libsonnet", ".want", ".wants":
			data = slices.Concat(
				[]byte(LocalGround(*fqp)),
				[]byte(LocalDerived(*fqp)),
				data,
			)
		}
		c := jsonnet.MakeContentsRaw(data)
		imp.cache[*fqp] = c
		return c, mkJsonnetPath(*fqp), nil
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
	return fmt.Sprintf(`local GROUND = {"module":"%v","derived":false,"callerPath":"%s"};`, fqp.ModuleCID.String(), fqp.Path)
}

func LocalDerived(fqp FQPath) string {
	return fmt.Sprintf(`local DERIVED = {"module":"%v","derived":true,"callerPath":"%s"};`, fqp.ModuleCID.String(), fqp.Path)
}

type FQPath struct {
	ModuleCID cadata.ID
	Path      string
}

func mkJsonnetPath(fqp FQPath) string {
	return fqp.ModuleCID.String() + ":" + fqp.Path
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
		ModuleCID: modCID,
		Path:      parts[1],
	}, nil
}
