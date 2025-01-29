package wantc

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/google/go-jsonnet"
	"wantbuild.io/want/src/wantcfg"
)

var _ jsonnet.Importer = &jsImporter{}

// vfsImporter is a jsonnet.Importer which imports from a build's VFS
type jsImporter struct {
	load func(p string) ([]byte, error)

	mu    sync.RWMutex
	cache map[string]jsonnet.Contents
}

func newVFSImporter(load func(p string) ([]byte, error)) *jsImporter {
	return &jsImporter{
		load:  load,
		cache: make(map[string]jsonnet.Contents),
	}
}

func (imp *jsImporter) Import(importedFrom, importedPath string) (contents jsonnet.Contents, foundAt string, err error) {
	switch importedPath {
	case "want", "wants":
		return libOnlyImporter{}.Import(importedFrom, importedPath)
	}
	p := PathFrom(importedFrom, importedPath)
	imp.mu.RLock()
	if contents, exists := imp.cache[p]; exists {
		imp.mu.RUnlock()
		return contents, p, nil
	}
	imp.mu.RUnlock()
	data, err := imp.load(p)
	if err != nil {
		return jsonnet.MakeContents(""), "", fmt.Errorf("importing %q from %q: %w", importedPath, importedFrom, err)
	}
	imp.mu.Lock()
	defer imp.mu.Unlock()
	if c, exists := imp.cache[p]; exists {
		return c, p, nil
	} else {
		c := jsonnet.MakeContents(string(data))
		imp.cache[p] = c
		return c, p, nil
	}
}

type libOnlyImporter struct{}

func (imp libOnlyImporter) Import(importedFrom, importedPath string) (contents jsonnet.Contents, foundAt string, err error) {
	if strings.Contains(importedPath, ":") {
		return jsonnet.Contents{}, "", fmt.Errorf("import path cannot contain ':'")
	}
	switch importedPath {
	case "want":
		c := jsonnet.MakeContents(wantcfg.LibWant(importedFrom))
		return c, "want:" + importedFrom, nil
	default:
		return jsonnet.MakeContents(""), "", fmt.Errorf("could not import %q", importedPath)
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
