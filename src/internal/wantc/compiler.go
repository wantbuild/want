package wantc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"
	"go.brendoncarroll.net/stdctx/logctx"
	"golang.org/x/sync/errgroup"

	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/internal/op/glfsops"
	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/internal/stringsets"
	"wantbuild.io/want/src/internal/wantdag"
	"wantbuild.io/want/src/wantcfg"
	"wantbuild.io/want/src/wantjob"
)

const (
	WantFilename       = "WANT"
	ExprPathSuffix     = ".want"
	StmtPathSuffix     = ".wants"
	MaxJsonnetFileSize = 1 << 20
)

type Metadata = map[string]any

type Compiler struct {
	glfs *glfs.Agent
}

func NewCompiler() *Compiler {
	return &Compiler{
		glfs: glfs.NewAgent(),
	}
}

type CompileTask struct {
	Metadata Metadata
	Module   glfs.Ref
	Deps     map[ExprID]glfs.Ref
}

func (c *Compiler) Compile(ctx context.Context, dst cadata.Store, src cadata.Getter, ct CompileTask) (*Plan, error) {
	isMod, err := IsModule(ctx, src, ct.Module)
	if err != nil {
		return nil, err
	}
	if !isMod {
		return nil, fmt.Errorf("not a want module")
	}
	return c.compileModule(ctx, dst, src, ct.Metadata, ct.Module, ct.Deps)
}

// compileCtx holds the state for a single run of the compiler
type compileCtx struct {
	ctx      context.Context
	src      cadata.Getter
	dst      cadata.Store
	buildCtx Metadata
	ground   glfs.Ref
	deps     map[ExprID]glfs.Ref

	jsImporter   *jsImporter
	vpMu         sync.Mutex
	visitedPaths map[string]chan struct{}
	vfsMu        sync.Mutex
	vfs          *VFS

	erMu      sync.Mutex
	exprRoots []*exprRoot
	ssMu      sync.Mutex
	stmtSets  []*stmtSet
	smMu      sync.Mutex
	subMods   []submodule

	knownMu  sync.Mutex
	known    []glfs.TreeEntry
	knownRef *glfs.Ref
	targets  []Target
}

func (cs *compileCtx) claimPath(p string) bool {
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

func (cs *compileCtx) awaitPath(ctx context.Context, p string) error {
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

func (cs *compileCtx) donePath(p string) {
	cs.vpMu.Lock()
	ch := cs.visitedPaths[p]
	cs.vpMu.Unlock()
	close(ch)
}

func (cs *compileCtx) appendExprRoot(er *exprRoot) {
	cs.erMu.Lock()
	defer cs.erMu.Unlock()
	cs.exprRoots = append(cs.exprRoots, er)
}

func (cs *compileCtx) appendStmtSet(ss *stmtSet) {
	cs.ssMu.Lock()
	defer cs.ssMu.Unlock()
	cs.stmtSets = append(cs.stmtSets, ss)
}

func (cc *compileCtx) appendSubmodule(sm submodule) {
	cc.smMu.Lock()
	defer cc.smMu.Unlock()
	cc.subMods = append(cc.subMods, sm)
}

func (cs *compileCtx) acquireVFS() (*VFS, func()) {
	cs.vfsMu.Lock()
	return cs.vfs, cs.vfsMu.Unlock
}

func (c *Compiler) compileModule(ctx context.Context, dst cadata.Store, src cadata.Getter, metadata Metadata, ground glfs.Ref, deps map[ModuleID]glfs.Ref) (*Plan, error) {
	cfg, err := GetModuleConfig(ctx, src, ground)
	if err != nil {
		return nil, err
	}
	for name, expr := range cfg.Namespace {
		eid := NewExprID(expr)
		if _, exists := deps[eid]; !exists {
			return nil, ErrMissingDep{Name: name}
		}
	}
	jsCtx, err := newJsonnetCtx(ctx, src, ground, deps)
	if err != nil {
		return nil, err
	}
	modIdx := map[ModuleID]glfs.Ref{}
	for modRef := range jsCtx.AllModules() {
		modIdx[NewModuleID(modRef)] = modRef
	}
	cs := &compileCtx{
		ctx:      ctx,
		src:      src,
		dst:      dst,
		buildCtx: metadata,
		ground:   ground,
		deps:     deps,

		vfs:          &VFS{},
		visitedPaths: make(map[string]chan struct{}),
		jsImporter: newImporter(jsCtx, func(fqp FQPath) ([]byte, error) {
			modRef, exists := modIdx[fqp.Module]
			if !exists {
				return nil, fmt.Errorf("cannot find module %v to import path %v", fqp.Module, fqp.Path)
			}
			ref := &modRef
			ref, err = c.glfs.GetAtPath(ctx, stores.Union{dst, src}, *ref, fqp.Path)
			if err != nil {
				return nil, err
			}
			return c.glfs.GetBlobBytes(ctx, stores.Union{dst, src}, *ref, MaxJsonnetFileSize)
		}),
	}
	for _, f := range []func(cc *compileCtx) error{
		c.addSourceFiles,
		c.checkStmts,
		c.detectCycles,
		c.lowerSelections,
		c.makeTargets,
		c.makeKnown,
	} {
		if err := f(cs); err != nil {
			return nil, err
		}
	}
	return &Plan{
		Known:   *cs.knownRef,
		Targets: cs.targets,
	}, nil
}

func (c *Compiler) addSourceFiles(cc *compileCtx) error {
	defer logStep(cc.ctx, "adding source files")()
	eg, _ := errgroup.WithContext(cc.ctx)
	if err := c.addSourceFile(cc, eg, "", 0o777, cc.ground); err != nil {
		return err
	}
	return eg.Wait()
}

func (c *Compiler) addSourceFile(cc *compileCtx, eg *errgroup.Group, p string, mode fs.FileMode, ref glfs.Ref) error {
	if gotIt := cc.claimPath(p); !gotIt {
		return cc.awaitPath(cc.ctx, p)
	}
	defer cc.donePath(p)
	switch ref.Type {
	case glfs.TypeTree:
		tree, err := c.glfs.GetTreeSlice(cc.ctx, cc.src, ref, 1e6)
		if err != nil {
			return err
		}
		for _, ent := range tree {
			ent := ent
			p2 := path.Join(p, ent.Name)
			if isMod, err := IsModule(cc.ctx, cc.src, ent.Ref); err != nil {
				return err
			} else if isMod {
				eg.Go(func() error {
					return c.addSubmodule(cc, eg, p2, ent.FileMode, ent.Ref)
				})
			} else {
				eg.Go(func() error {
					return c.addSourceFile(cc, eg, p2, ent.FileMode, ent.Ref)
				})
			}
		}

	case glfs.TypeBlob:
		ks := stringsets.Unit(p)
		switch {
		case IsExprFilePath(p):
			return c.loadExprFile(cc, p)
		case IsStmtFilePath(p):
			return c.loadStmtFile(cc, p)
		default:
			if err := glfstasks.FastSync(cc.ctx, cc.dst, cc.src, ref); err != nil {
				return err
			}
			cc.knownMu.Lock()
			cc.known = append(cc.known, glfs.TreeEntry{
				Name:     p,
				FileMode: mode,
				Ref:      ref,
			})
			cc.knownMu.Unlock()
		}
		vfs, rel := cc.acquireVFS()
		defer rel()
		return vfs.Add(VFSEntry{
			K:       ks,
			V:       &value{ref: ref},
			PlaceAt: p,
		})
	}
	return nil
}

func (c *Compiler) addSubmodule(cc *compileCtx, eg *errgroup.Group, p string, mode fs.FileMode, modRef glfs.Ref) error {
	sm := newSubmodule(p, modRef)
	cc.appendSubmodule(sm)
	return c.addSourceFile(cc, eg, p, mode, modRef)
}

func (c *Compiler) loadExprFile(cc *compileCtx, p string) error {
	fqp := FQPath{Module: cc.ground.CID, Path: p}
	er, err := c.parseExprRoot(cc, fqp)
	if err != nil {
		return err
	}
	ks := er.Affects()
	vfs, rel := cc.acquireVFS()
	defer rel()
	if err := vfs.Add(VFSEntry{K: ks, V: er.expr, PlaceAt: p, DefinedIn: fqp.Path}); err != nil {
		panic(err)
	}
	cc.appendExprRoot(er)
	return nil
}

func (c *Compiler) loadStmtFile(cc *compileCtx, p string) error {
	fqp := FQPath{Module: cc.ground.CID, Path: p}
	ss, err := c.parseStmtSet(cc, fqp)
	if err != nil {
		return err
	}
	for i, stmt := range ss.stmts {
		ks := stmt.Affects()
		allowed := stringsets.Prefix(strings.TrimLeft(parentPath(p)+"/", "/"))
		if !stringsets.Subset(ks, allowed) {
			return fmt.Errorf("statement files can only affect the immediate parent tree and below, but %s affects %v", p, ks)
		}
		vfs, rel := cc.acquireVFS()
		if err := vfs.Add(VFSEntry{K: ks, V: stmt.expr(), DefinedIn: p}); err != nil {
			rel()
			return fmt.Errorf("statement %d in file %q, outputs to conflicted keyspace %w", i, p, err)
		}
		rel()
	}
	cc.appendStmtSet(ss)
	return nil
}

func (c *Compiler) checkStmts(cc *compileCtx) error {
	defer logStep(cc.ctx, "checking statements")()
	for _, ss := range cc.stmtSets {
		for i, stmt := range ss.stmts {
			for _, sm := range cc.subMods {
				if strings.HasPrefix(ss.path, sm.p) {
					continue // if they are in the same submodule, that's OK.
				}
				subModSet := stringsets.Prefix(sm.p)
				if stringsets.Intersects(subModSet, stmt.Dst) {
					return ErrSubmoduleConflict{
						DefinedIn:  ss.path,
						DefinedNum: i,
						Submodule:  sm.p,
					}
				}
			}
		}
	}
	return nil
}

// lowerSelections turns all selections in all the expressions into compute and fact
func (c *Compiler) lowerSelections(cc *compileCtx) error {
	defer logStep(cc.ctx, "lowering selections")()
	cache := make(map[[32]byte]Expr)
	vfs, rel := cc.acquireVFS()
	defer rel()
	// TODO: track per-module state
	vfss := map[ModuleID]*VFS{
		NewModuleID(cc.ground): vfs,
	}
	for i, er := range cc.exprRoots {
		e2, err := c.replaceSelections(cc, cache, vfss, er.expr)
		if err != nil {
			return err
		}
		cc.exprRoots[i].expr = e2
	}
	for _, ss := range cc.stmtSets {
		for _, stmt := range ss.stmts {
			e1 := stmt.expr()
			e2, err := c.replaceSelections(cc, cache, vfss, e1)
			if err != nil {
				return err
			}
			stmt.setExpr(e2)
		}
	}
	return nil
}

func (c *Compiler) replaceSelections(cc *compileCtx, cache map[[32]byte]Expr, vfss map[ModuleID]*VFS, expr Expr) (ret Expr, retErr error) {
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
			e2, err := c.replaceSelections(cc, cache, vfss, input.From)
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
		vfs, exists := vfss[x.module]
		if !exists {
			return nil, fmt.Errorf("missing module for selection %v", x)
		}
		var e Expr
		if x.derived {
			var err error
			if e, err = c.query(cc.ctx, cc.dst, vfs, x.set); err != nil {
				return nil, err
			}
		} else {
			var err error
			if e, err = c.selectGround(cc, x.module, x.set); err != nil {
				return nil, err
			}
		}
		return c.replaceSelections(cc, cache, vfss, e)
	case *value:
		return x, nil
	default:
		panic(x)
	}
}

func (c *Compiler) detectCycles(cc *compileCtx) error {
	defer logStep(cc.ctx, "detecting cycles")()
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
		for _, ent := range cc.vfs.Get(x) {
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
	for _, er := range cc.exprRoots {
		if !traverseExpr(er.expr) {
			if err := check(); err != nil {
				return err
			}
		}
	}
	for _, ss := range cc.stmtSets {
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

func (c *Compiler) makeTargets(cc *compileCtx) error {
	defer logStep(cc.ctx, "making graph")()
	var targets []Target

	// ExprRoots
	for _, er := range cc.exprRoots {
		dag, err := c.makeGraphFromExpr(cc.ctx, cc.dst, cc.src, er.expr, er.path)
		if err != nil {
			return err
		}
		targets = append(targets, Target{
			To:   stringsets.ToPathSet(er.Affects()),
			DAG:  *dag,
			Expr: er.spec,

			DefinedIn: er.path,
		})
	}

	// StmtSets
	for _, ss := range cc.stmtSets {
		for i, stmt := range ss.stmts {
			to := stringsets.ToPathSet(stmt.Affects())
			expr, err := c.filterExpr(cc.ctx, cc.dst, stmt.expr(), to)
			if err != nil {
				return err
			}
			dag, err := c.makeGraphFromExpr(cc.ctx, cc.dst, cc.src, expr, "")
			if err != nil {
				return err
			}
			targets = append(targets, Target{
				To:   to,
				DAG:  *dag,
				Expr: ss.specs[i].Expr(),

				IsStatement: true,
				DefinedIn:   ss.path,
				DefinedNum:  i,
			})
		}
	}

	slices.SortFunc(targets, func(a, b Target) int {
		if a.DefinedIn != b.DefinedIn {
			return strings.Compare(a.DefinedIn, b.DefinedIn)
		}
		return a.DefinedNum - b.DefinedNum
	})
	cc.targets = targets
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

func (c *Compiler) makeKnown(cc *compileCtx) error {
	cc.knownMu.Lock()
	defer cc.knownMu.Unlock()
	slices.SortFunc(cc.known, func(a, b glfs.TreeEntry) int {
		return strings.Compare(a.Name, b.Name)
	})
	ref, err := c.glfs.PostTreeSlice(cc.ctx, cc.dst, cc.known)
	if err != nil {
		return err
	}
	if err := glfstasks.FastSync(cc.ctx, cc.dst, cc.src, *ref); err != nil {
		return err
	}
	cc.knownRef = ref
	return nil
}

func (c *Compiler) pickExpr(ctx context.Context, dst cadata.Store, x Expr, p string) (Expr, error) {
	if p == "" {
		return x, nil
	}
	ref, err := c.glfs.PostBlob(ctx, dst, strings.NewReader(p))
	if err != nil {
		return nil, err
	}
	pathExpr := &value{ref: *ref}
	return &compute{
		Op: wantjob.OpName("glfs.") + glfsops.OpPick,
		Inputs: []computeInput{
			{To: "x", From: x},
			{To: "path", From: pathExpr},
		},
	}, nil
}

func (c *Compiler) filterExpr(ctx context.Context, dst cadata.PostExister, x Expr, q wantcfg.PathSet) (Expr, error) {
	data, err := json.Marshal(q)
	if err != nil {
		return nil, err
	}
	ref, err := c.glfs.PostBlob(ctx, dst, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	filterExpr := &value{*ref}
	return &compute{
		Op: wantjob.OpName("glfs.") + glfsops.OpFilterPathSet,
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

func logStep(ctx context.Context, step string) func() {
	logctx.Debugf(ctx, "%s: begin", step)
	return func() { logctx.Debugf(ctx, "%s: done", step) }
}

type submodule struct {
	p    string
	root glfs.Ref
}

func newSubmodule(p string, root glfs.Ref) submodule {
	return submodule{p: p, root: root}
}
