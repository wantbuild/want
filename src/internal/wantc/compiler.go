package wantc

import (
	"context"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"
	"go.brendoncarroll.net/stdctx/logctx"
	"golang.org/x/sync/errgroup"

	"wantbuild.io/want/src/internal/op/glfsops"
	"wantbuild.io/want/src/internal/stringsets"
	"wantbuild.io/want/src/internal/wantdag"
)

const (
	WantFilename       = "WANT"
	ExprPathSuffix     = ".want"
	StmtPathSuffix     = ".wants"
	MaxJsonnetFileSize = 1 << 20
)

type Metadata = map[string]any

func AddGitMetadata(buildCtx map[string]any, commitHash string, tags []string) {
	buildCtx["gitCommitHash"] = commitHash
	buildCtx["gitTags"] = tags
}

type Compiler struct {
	glfs *glfs.Agent
}

func NewCompiler() *Compiler {
	return &Compiler{
		glfs: glfs.NewAgent(),
	}
}

type CompileTask struct {
	Metadata  Metadata
	Module    glfs.Ref
	Deps      map[ModuleID]glfs.Ref
	Namespace Namespace
}

func (c *Compiler) Compile(ctx context.Context, dst cadata.Store, src cadata.Getter, ct CompileTask) (*Plan, error) {
	isMod, err := IsModule(ctx, src, ct.Module)
	if err != nil {
		return nil, err
	}
	if !isMod {
		return nil, fmt.Errorf("not a want module")
	}
	return c.compileModule(ctx, dst, src, ct.Metadata, ct.Module, ct.Namespace, ct.Deps)
}

// compileState holds the state for a single run of the compiler
type compileState struct {
	src      cadata.Getter
	dst      cadata.Store
	buildCtx Metadata
	ground   glfs.Ref

	jsImporter   *jsImporter
	vpMu         sync.Mutex
	visitedPaths map[string]chan struct{}
	vfsMu        sync.Mutex
	vfs          *VFS
	erMu         sync.Mutex
	exprRoots    []*ExprRoot
	ssMu         sync.Mutex
	stmtSets     []*StmtSet

	knownMu  sync.Mutex
	known    []glfs.TreeEntry
	knownRef *glfs.Ref
	targets  []Target
}

func (cs *compileState) claimPath(p string) bool {
	cs.vpMu.Lock()
	defer cs.vpMu.Unlock()
	_, exists := cs.visitedPaths[p]
	if !exists {
		cs.visitedPaths[p] = make(chan struct{})
		return true
	} else {
		return false
	}
}

func (cs *compileState) awaitPath(ctx context.Context, p string) error {
	cs.vpMu.Lock()
	ch, exists := cs.visitedPaths[p]
	if !exists {
		panic(p)
	}
	cs.vpMu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ch:
		return nil
	}
}

func (cs *compileState) donePath(p string) {
	cs.vpMu.Lock()
	ch := cs.visitedPaths[p]
	cs.vpMu.Unlock()
	close(ch)
}

func (cs *compileState) appendExprRoot(er *ExprRoot) {
	cs.erMu.Lock()
	defer cs.erMu.Unlock()
	cs.exprRoots = append(cs.exprRoots, er)
}

func (cs *compileState) appendStmtSet(ss *StmtSet) {
	cs.ssMu.Lock()
	defer cs.ssMu.Unlock()
	cs.stmtSets = append(cs.stmtSets, ss)
}

func (cs *compileState) acquireVFS() (*VFS, func()) {
	cs.vfsMu.Lock()
	return cs.vfs, cs.vfsMu.Unlock
}

func (c *Compiler) compileModule(ctx context.Context, dst cadata.Store, src cadata.Getter, metadata Metadata, ground glfs.Ref, ns Namespace, deps map[ModuleID]glfs.Ref) (*Plan, error) {
	cfg, err := GetModuleConfig(ctx, src, ground)
	if err != nil {
		return nil, err
	}
	// check namespace
	for name := range cfg.Namespace {
		if _, exists := ns[name]; !exists {
			return nil, ErrMissingDep{Name: name}
		}
	}
	for name := range ns {
		if _, exists := cfg.Namespace[name]; !exists {
			return nil, ErrExtraDep{Name: name}
		}
	}
	deps = maps.Clone(deps)
	if deps == nil {
		deps = make(map[cadata.ID]glfs.Ref)
	}
	for _, ref := range ns {
		if ref.Type != glfs.TypeTree {
			deps[NewModuleID(ref)] = ref
		}
	}
	if _, exists := deps[NewModuleID(ground)]; exists {
		// This is a bug in the want.build Operation
		return nil, fmt.Errorf("module deps must not contain the main module")
	}
	cs := &compileState{
		src:          src,
		dst:          dst,
		buildCtx:     metadata,
		ground:       ground,
		vfs:          &VFS{},
		visitedPaths: make(map[string]chan struct{}),
		jsImporter: newImporter(func(modid ModuleID, k string) (ModuleID, error) {
			if modid == ground.CID {
				if dep, ok := ns[k]; ok {
					return dep.CID, nil
				} else {
					return ModuleID{}, fmt.Errorf("%v not found in namespace", k)
				}
			}
			// TODO: need to also resolve namespace entries in dependency modules
			return ModuleID{}, fmt.Errorf("cannot resolve %v", modid)
		},
			func(fqp FQPath) ([]byte, error) {
				modRef, exists := deps[fqp.ModuleID]
				if !exists && fqp.ModuleID == NewModuleID(ground) {
					modRef = ground
				} else if !exists {
					return nil, fmt.Errorf("cannot find module %v to import path %v", fqp.ModuleID, fqp.Path)
				}
				p := fqp.Path
				if after, yes := strings.CutPrefix(p, "@"); yes {
					parts := strings.SplitN(after, "/", 2)
					modRef = ns[parts[0]]
					if len(parts) > 1 {
						p = parts[1]
					} else {
						p = ""
					}
				}
				ref := &modRef
				if p != "" {
					ref, err = c.glfs.GetAtPath(ctx, src, *ref, p)
					if err != nil {
						return nil, err
					}
				}
				return c.glfs.GetBlobBytes(ctx, src, *ref, MaxJsonnetFileSize)
			}),
	}
	for _, f := range []func(context.Context, *compileState) error{
		c.addSourceFiles,
		c.detectCycles,
		c.lowerSelections,
		c.makeTargets,
		c.makeKnown,
	} {
		if err := f(ctx, cs); err != nil {
			return nil, err
		}
	}
	return &Plan{
		Known:   *cs.knownRef,
		Targets: cs.targets,
	}, nil
}

func (c *Compiler) addSourceFiles(ctx context.Context, cs *compileState) error {
	defer logStep(ctx, "adding source files")()
	eg, _ := errgroup.WithContext(ctx)
	if err := c.addSourceFile(ctx, cs, eg, "", 0o777, cs.ground); err != nil {
		return err
	}
	return eg.Wait()
}

func (c *Compiler) addSourceFile(ctx context.Context, cs *compileState, eg *errgroup.Group, p string, mode fs.FileMode, ref glfs.Ref) error {
	if gotIt := cs.claimPath(p); !gotIt {
		return cs.awaitPath(ctx, p)
	}
	defer cs.donePath(p)
	switch ref.Type {
	case glfs.TypeTree:
		tree, err := c.glfs.GetTreeSlice(ctx, cs.src, ref, 1e6)
		if err != nil {
			return err
		}
		if p != "" {
			if isMod, err := IsModule(ctx, cs.src, ref); err != nil {
				return err
			} else if isMod {
				return fmt.Errorf("submodules not yet supported.  Found submodule at path %s", p)
			}
		}
		for _, ent := range tree {
			ent := ent
			p2 := path.Join(p, ent.Name)
			eg.Go(func() error {
				return c.addSourceFile(ctx, cs, eg, p2, ent.FileMode, ent.Ref)
			})
		}

	case glfs.TypeBlob:
		ks := stringsets.Unit(p)
		switch {
		case p == "WANT":
			// Drop the project configuration file from the build.
		case IsExprFilePath(p):
			return c.loadExpr(ctx, cs, p)
		case IsStmtFilePath(p):
			return c.loadStmt(ctx, cs, p)
		default:
			if err := glfs.Sync(ctx, cs.dst, cs.src, ref); err != nil {
				return err
			}
			cs.knownMu.Lock()
			cs.known = append(cs.known, glfs.TreeEntry{
				Name:     p,
				FileMode: mode,
				Ref:      ref,
			})
			cs.knownMu.Unlock()
		}
		vfs, rel := cs.acquireVFS()
		defer rel()
		if !isExportAt(vfs, ks) {
			return vfs.Add(VFSEntry{
				K: ks,
				V: &value{ref: ref},
			})
		}
	}
	return nil
}

func (c *Compiler) loadExpr(ctx context.Context, cs *compileState, p string) error {
	fqp := FQPath{ModuleID: cs.ground.CID, Path: p}
	er, err := c.parseExprRoot(ctx, cs, fqp)
	if err != nil {
		return err
	}
	ks := er.Affects()
	vfs, rel := cs.acquireVFS()
	defer rel()
	if err := vfs.Add(VFSEntry{K: ks, V: er.expr, DefinedIn: fqp.Path}); err != nil {
		panic(err)
	}
	cs.appendExprRoot(er)
	return nil
}

func (c *Compiler) loadStmt(ctx context.Context, cs *compileState, p string) error {
	fqp := FQPath{ModuleID: cs.ground.CID, Path: p}
	ss, err := c.parseStmtSet(ctx, cs, fqp)
	if err != nil {
		return err
	}
	for i, stmt := range ss.stmts {
		ks := stmt.Affects()
		allowed := stringsets.Prefix(parentPath(p) + "/")
		if !stringsets.Subset(ks, allowed) {
			return fmt.Errorf("statement files can only affect the immediate parent tree and below, but %s affects %v", p, ks)
		}
		vfs, rel := cs.acquireVFS()
		// we can overwrite a single fact, but nothing else.
		_, isExport := stmt.(*exportStmt)
		if isExport && isFactAt(vfs, ks) {
			cs.vfs.Delete(ks)
		}
		if err := vfs.Add(VFSEntry{K: ks, V: stmt.expr(), DefinedIn: p, IsExport: isExport}); err != nil {
			rel()
			return fmt.Errorf("statement %d in file %q, outputs to conflicted keyspace %w", i, p, err)
		}
		rel()
	}
	cs.appendStmtSet(ss)
	return nil
}

// lowerSelections turns all selections in all the expressions into compute and fact
func (c *Compiler) lowerSelections(ctx context.Context, cs *compileState) error {
	defer logStep(ctx, "lowering selections")()
	cache := make(map[[32]byte]Expr)
	vfs, rel := cs.acquireVFS()
	defer rel()
	for i, er := range cs.exprRoots {
		e2, err := c.replaceSelections(ctx, cs.dst, cache, vfs, er.expr)
		if err != nil {
			return err
		}
		cs.exprRoots[i].expr = e2
	}
	for _, ss := range cs.stmtSets {
		for _, stmt := range ss.stmts {
			e1 := stmt.expr()
			e2, err := c.replaceSelections(ctx, cs.dst, cache, vfs, e1)
			if err != nil {
				return err
			}
			stmt.setExpr(e2)
		}
	}
	return nil
}

func (c *Compiler) replaceSelections(ctx context.Context, dst cadata.Store, cache map[[32]byte]Expr, vfs *VFS, expr Expr) (ret Expr, retErr error) {
	if y, exists := cache[expr.Key()]; exists {
		return y, nil
	}
	defer func() {
		if retErr == nil {
			cache[expr.Key()] = ret
		}
	}()
	switch x := expr.(type) {
	case *compute:
		var yInputs []computeInput
		for _, input := range x.Inputs {
			e2, err := c.replaceSelections(ctx, dst, cache, vfs, input.From)
			if err != nil {
				return nil, err
			}
			yInputs = append(yInputs, computeInput{
				To:   input.To,
				From: e2,
				Mode: input.Mode,
			})
		}
		return &compute{
			Op:     x.Op,
			Inputs: yInputs,
		}, nil
	case *selection:
		e, err := c.query(ctx, dst, vfs, x.set, x.pick)
		if err != nil {
			return nil, err
		}
		return c.replaceSelections(ctx, dst, cache, vfs, e)
	case *value:
		return x, nil
	default:
		panic(x)
	}
}

func (c *Compiler) detectCycles(ctx context.Context, cs *compileState) error {
	defer logStep(ctx, "detecting cycles")()
	visited := make(map[string]struct{})
	current := make(map[string]struct{})
	var path []string
	// dfs returns false to abort search early
	var dfs func(stringsets.Set) bool
	var traverseExpr func(Expr) bool
	traverseExpr = func(x Expr) bool {
		switch x := x.(type) {
		case *selection:
			if !dfs(x.set) {
				return false
			}
		case *compute:
			for _, in := range x.Inputs {
				if !traverseExpr(in.From) {
					return false
				}
			}
		}
		return true
	}
	dfs = func(x stringsets.Set) bool {
		// don't visit more than once
		if _, exists := visited[x.String()]; exists {
			return true
		}

		// cycle detection
		if _, exists := current[x.String()]; exists {
			return false
		}
		current[x.String()] = struct{}{}
		path = append(path, x.String())
		defer delete(current, x.String())
		defer func() { path = path[:len(path)-1] }()
		for _, ent := range cs.vfs.Get(x) {
			if !traverseExpr(ent.V) {
				return false
			}
		}
		// mark visited
		visited[x.String()] = struct{}{}
		return true
	}
	check := func() error {
		if len(path) != 0 {
			return ErrCycle{Cycle: path}
		}
		return nil
	}
	for _, er := range cs.exprRoots {
		if !traverseExpr(er.expr) {
			if err := check(); err != nil {
				return err
			}
		}
	}
	for _, ss := range cs.stmtSets {
		for _, stmt := range ss.stmts {
			if !traverseExpr(stmt.expr()) {
				if err := check(); err != nil {
					return err
				}
			}
		}
	}
	return check()
}

func (c *Compiler) makeTargets(ctx context.Context, cs *compileState) error {
	defer logStep(ctx, "making graph")()
	var targets []Target

	// ExprRoots
	for _, er := range cs.exprRoots {
		dag, err := c.makeGraphFromExpr(ctx, cs.dst, cs.src, er.expr, er.path)
		if err != nil {
			return err
		}
		targets = append(targets, Target{
			To:   makePathSet(er.Affects()),
			DAG:  *dag,
			Expr: er.spec,

			DefinedIn: er.path,
		})
	}

	// StmtSets
	for _, ss := range cs.stmtSets {
		for i, stmt := range ss.stmts {
			aff := stmt.Affects()
			placeAt := stringsets.BoundingPrefix(aff)
			dag, err := c.makeGraphFromExpr(ctx, cs.dst, cs.src, stmt.expr(), placeAt)
			if err != nil {
				return err
			}
			_, isExport := stmt.(*exportStmt)
			to := makePathSet(stmt.Affects())
			targets = append(targets, Target{
				To:   to,
				DAG:  *dag,
				Expr: ss.specs[i].Expr(),

				IsStatement: true,
				DefinedIn:   ss.path,
				DefinedNum:  i,
				IsExport:    isExport,
			})
		}
	}
	slices.SortFunc(targets, func(a, b Target) int {
		if a.DefinedIn != b.DefinedIn {
			return strings.Compare(a.DefinedIn, b.DefinedIn)
		}
		return a.DefinedNum - b.DefinedNum
	})
	cs.targets = targets
	return nil
}

func (c *Compiler) makeGraphFromExpr(ctx context.Context, dst cadata.Store, src cadata.Getter, x Expr, place string) (*glfs.Ref, error) {
	gb := NewGraphBuilder(dst)
	nid, err := gb.Expr(ctx, src, x)
	if err != nil {
		return nil, err
	}
	if _, err := gb.place(ctx, place, nid); err != nil {
		return nil, err
	}
	return wantdag.PostDAG(ctx, dst, gb.Finish())
}

func (c *Compiler) makeKnown(ctx context.Context, cs *compileState) error {
	cs.knownMu.Lock()
	defer cs.knownMu.Unlock()
	slices.SortFunc(cs.known, func(a, b glfs.TreeEntry) int {
		return strings.Compare(a.Name, b.Name)
	})
	ref, err := c.glfs.PostTreeSlice(ctx, cs.dst, cs.known)
	if err != nil {
		return err
	}
	if err := glfs.Sync(ctx, cs.dst, cs.src, *ref); err != nil {
		return err
	}
	cs.knownRef = ref
	return nil
}

func (c *Compiler) pickExpr(ctx context.Context, dst cadata.Store, x Expr, p string) (Expr, error) {
	ref, err := c.glfs.PostBlob(ctx, dst, strings.NewReader(p))
	if err != nil {
		return nil, err
	}
	pathExpr := &value{ref: *ref}
	return &compute{
		Op: glfsops.OpPick,
		Inputs: []computeInput{
			{To: "x", From: x},
			{To: "path", From: pathExpr},
		},
	}, nil
}

func (c *Compiler) filterExpr(ctx context.Context, dst cadata.Store, x Expr, re *regexp.Regexp) (Expr, error) {
	ref, err := c.glfs.PostBlob(ctx, dst, strings.NewReader(re.String()))
	if err != nil {
		return nil, err
	}
	filterExpr := &value{*ref}
	return &compute{
		Op: glfsops.OpFilter,
		Inputs: []computeInput{
			{To: "x", From: x},
			{To: "filter", From: filterExpr},
		},
	}, nil
}

func PathFrom(from, p string) string {
	if strings.HasPrefix(p, "../") {
		return PathFrom(path.Dir(from), p[3:])
	}
	if strings.HasPrefix(p, "./") {
		return path.Join(path.Dir(from), p[2:])
	}
	return p
}

func IsRelativePath(p string) bool {
	return strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../")
}

func IsExprFilePath(p string) bool {
	return strings.HasSuffix(p, ExprPathSuffix) || strings.HasSuffix(p, ExprPathSuffix+".jsonnet")
}

func IsStmtFilePath(p string) bool {
	return strings.HasSuffix(p, StmtPathSuffix)
}

func parentPath(p string) string {
	p = glfs.CleanPath(p)
	parts := strings.Split(p, "/")
	return glfs.CleanPath(strings.Join(parts[:len(parts)-1], "/"))
}

func isExportAt(vfs *VFS, ks stringsets.Set) bool {
	ents := vfs.Get(ks)
	return len(ents) == 1 && ents[0].IsExport
}

func isFactAt(vfs *VFS, ks stringsets.Set) bool {
	ents := vfs.Get(ks)
	if len(ents) == 1 {
		_, ok := ents[0].V.(*value)
		return ok
	}
	return false
}

func logStep(ctx context.Context, step string) func() {
	logctx.Debugf(ctx, "%s: begin", step)
	return func() { logctx.Debugf(ctx, "%s: done", step) }
}
