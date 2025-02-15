package wantc

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/blobcache/glfs"
	"github.com/google/go-jsonnet"
	"go.brendoncarroll.net/state/cadata"
)

var _ jsonnet.Importer = &jsImporter{}

// jsImporter is a jsonnet.Importer which imports from a build's VFS
type jsImporter struct {
	ctxs map[ModuleID]*jsonnetCtx
	load func(FQPath) ([]byte, error)

	mu    sync.RWMutex
	cache map[FQPath]jsonnet.Contents
}

func newImporter(jc *jsonnetCtx, load func(fqp FQPath) ([]byte, error)) *jsImporter {
	ctxs := map[ModuleID]*jsonnetCtx{}
	var index func(jc *jsonnetCtx)
	index = func(jc *jsonnetCtx) {
		if _, exists := ctxs[jc.ModuleID()]; exists {
			return
		}
		for _, jc2 := range jc.Namespace {
			index(jc2)
		}
		for _, jc2 := range jc.Submodules {
			index(jc2)
		}
		ctxs[jc.ModuleID()] = jc
	}
	index(jc)
	return &jsImporter{
		ctxs: ctxs,
		load: load,

		cache: make(map[FQPath]jsonnet.Contents),
	}
}

func (imp *jsImporter) Import(importedFrom, importedPath string) (contents jsonnet.Contents, foundAt string, err error) {
	// figure out where the import is from, and what it is asking for
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
		targetFQP = FQPath{Module: fromFQP.Module, Path: importedPath}
	}
	// resolve the import
	jc := imp.ctxs[targetFQP.Module]
	targetFQP, err = jc.Resolve(fromFQP.Path, targetFQP.Path)
	if err != nil {
		return jsonnet.Contents{}, "", err
	}
	// now we know what we are looking for
	// first, check the cache
	imp.mu.RLock()
	if contents, exists := imp.cache[targetFQP]; exists {
		imp.mu.RUnlock()
		return contents, mkJsonnetPath(targetFQP), nil
	}
	imp.mu.RUnlock()
	// not in the cache, so load
	data, err := imp.load(targetFQP)
	if err != nil {
		return jsonnet.Contents{}, "", fmt.Errorf("importing %q from %q: %w", importedPath, importedFrom, err)
	}
	// put it back in the cache, but check if someone else beat us to it.
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
	return fmt.Sprintf(`local GROUND = {"__type__":"source","module":"%v","derived":false,"callerPath":"%s"};`, fqp.Module.String(), fqp.Path)
}

func LocalDerived(fqp FQPath) string {
	return fmt.Sprintf(`local DERIVED = {"__type__":"source","module":"%v","derived":true,"callerPath":"%s"};`, fqp.Module.String(), fqp.Path)
}

func mkJsonnetPath(fqp FQPath) string {
	return fqp.Module.String() + ":" + fqp.Path
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
		Module: modCID,
		Path:   parts[1],
	}, nil
}

type jsonnetCtx struct {
	Root glfs.Ref
	// Namespace entries are imported with @
	Namespace map[string]*jsonnetCtx
	// Submodules are imported normally, but require a context shift
	Submodules map[string]*jsonnetCtx
}

func newJsonnetCtx(ctx context.Context, src cadata.Getter, root glfs.Ref, deps map[ExprID]glfs.Ref) (*jsonnetCtx, error) {
	if root.Type != glfs.TypeTree {
		return &jsonnetCtx{Root: root}, nil
	}
	// namespace
	modCfg, err := GetModuleConfig(ctx, src, root)
	if err != nil {
		return nil, err
	}
	namespace := map[string]*jsonnetCtx{}
	for name, expr := range modCfg.Namespace {
		ref, exists := deps[NewExprID(expr)]
		if !exists {
			return nil, fmt.Errorf("missing ns entry for %s", name)
		}
		jctx, err := newJsonnetCtx(ctx, src, ref, deps)
		if err != nil {
			return nil, err
		}
		namespace[name] = jctx
	}

	// submodules
	mods, err := FindModules(ctx, src, root)
	if err != nil {
		return nil, err
	}
	delete(mods, "") // only submodules after this
	submods := map[string]*jsonnetCtx{}
	for prefix, submod := range mods {
		jctx, err := newJsonnetCtx(ctx, src, submod, deps)
		if err != nil {
			return nil, err
		}
		submods[prefix] = jctx
	}
	return &jsonnetCtx{
		Root:       root,
		Namespace:  namespace,
		Submodules: submods,
	}, nil
}

func (jc *jsonnetCtx) AllModules() iter.Seq[glfs.Ref] {
	return func(yield func(glfs.Ref) bool) {
		if !yield(jc.Root) {
			return
		}
		for _, jc2 := range jc.Namespace {
			for ref := range jc2.AllModules() {
				if !yield(ref) {
					return
				}
			}
		}
		for _, jc2 := range jc.Submodules {
			for ref := range jc2.AllModules() {
				if !yield(ref) {
					return
				}
			}
		}
	}
}

func (jc *jsonnetCtx) ModuleID() ModuleID {
	return NewModuleID(jc.Root)
}

func (jc *jsonnetCtx) Resolve(from, p string) (FQPath, error) {
	// namespace
	if after, ok := strings.CutPrefix(p, "@"); ok {
		parts := strings.SplitN(after, "/", 2)
		name := parts[0]
		jc2, exists := jc.Namespace[name]
		if !exists {
			return FQPath{}, fmt.Errorf("no namespace entry for %s", name)
		}
		if len(parts) < 2 {
			parts = append(parts, "")
		}
		return jc2.Resolve("", parts[1])
	}
	p = PathFrom(from, p)
	// submodules
	for prefix, submod := range jc.Submodules {
		if after, ok := strings.CutPrefix(p, prefix+"/"); ok {
			return submod.Resolve("", after)
		}
	}
	// local
	return FQPath{Module: jc.ModuleID(), Path: p}, nil
}
