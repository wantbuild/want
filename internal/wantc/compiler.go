package wantc

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"
	"go.brendoncarroll.net/stdctx/logctx"
	"golang.org/x/sync/errgroup"

	"wantbuild.io/want/internal/op/glfsops"
	"wantbuild.io/want/internal/stringsets"
	"wantbuild.io/want/internal/wantdag"
)

const (
	ExprPathSuffix     = ".want"
	StmtPathSuffix     = ".wants"
	MaxJsonnetFileSize = 1e6
)

type Compiler struct {
	glfs  *glfs.Agent
	store cadata.Store
}

func NewCompiler(store cadata.Store) *Compiler {
	return &Compiler{
		glfs:  glfs.NewAgent(),
		store: store,
	}
}

// compileState holds the state for a single run of the compiler
type compileState struct {
	src    cadata.Getter
	ground glfs.Ref
	root   Expr
	config compileConfig

	jsImporter   *jsImporter
	vpMu         sync.Mutex
	visitedPaths map[string]chan struct{}
	vfsMu        sync.Mutex
	vfs          *VFS
	erMu         sync.Mutex
	exprRoots    []*ExprRoot
	ssMu         sync.Mutex
	stmtSets     []*StmtSet

	graph     glfs.Ref
	nodeCount uint64
	targets   []Target
	rootNode  wantdag.NodeID
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

type compileConfig struct {
	metadata map[string]any
}

func (cc *compileConfig) setInputRef(x glfs.Ref) {
	if cc.metadata == nil {
		cc.metadata = make(map[string]any)
	}
	cc.metadata["inputRef"] = x
}

func collectCompileConfig(opts []CompileOption) compileConfig {
	ret := &compileConfig{}
	for _, o := range opts {
		o(ret)
	}
	return *ret
}

type CompileOption func(*compileConfig)

func WithGitMetadata(commitHash, tag string) CompileOption {
	return func(co *compileConfig) {
		if co.metadata == nil {
			co.metadata = make(map[string]any)
		}
		co.metadata["gitCommitHash"] = commitHash
		co.metadata["gitTag"] = tag
	}
}

func (c *Compiler) Compile(ctx context.Context, src cadata.Getter, ground glfs.Ref, prefix string, opts ...CompileOption) (*Plan, error) {
	cfg := collectCompileConfig(opts)
	cfg.setInputRef(ground)

	// TODO: remove, only copy to dst as needed
	if err := glfs.Sync(ctx, c.store, src, ground); err != nil {
		return nil, err
	}

	cs := &compileState{
		src:          src,
		config:       cfg,
		ground:       ground,
		root:         &selection{set: stringsets.Prefix(prefix)},
		vfs:          &VFS{},
		visitedPaths: make(map[string]chan struct{}),
	}

	if _, err := glfs.GetAtPath(ctx, c.store, ground, "WANT"); err != nil {
		return nil, fmt.Errorf("error accessing WANT file. %w", err)
	}
	for _, f := range []func(context.Context, *compileState, string) error{
		c.addSourceFiles,
		c.detectCycles,
		c.lowerSelections,
		c.makeGraph,
	} {
		if err := f(ctx, cs, prefix); err != nil {
			return nil, err
		}
	}
	return &Plan{
		Graph:     cs.graph,
		Root:      cs.rootNode,
		VFS:       cs.vfs,
		Targets:   cs.targets,
		NodeCount: cs.nodeCount,
	}, nil
}

func (c *Compiler) addSourceFiles(ctx context.Context, cs *compileState, p string) error {
	defer logStep(ctx, "adding source files")()
	eg, ctx := errgroup.WithContext(ctx)
	cs.jsImporter = newVFSImporter(func(p string) ([]byte, error) {
		ref, err := c.glfs.GetAtPath(ctx, c.store, cs.ground, p)
		if err != nil {
			return nil, err
		}
		return c.glfs.GetBlobBytes(ctx, c.store, *ref, MaxJsonnetFileSize)
	})
	ref, err := c.glfs.GetAtPath(ctx, c.store, cs.ground, p)
	if err != nil {
		return err
	}
	if err := c.addSourceFile(ctx, cs, eg, p, *ref); err != nil {
		return err
	}
	return eg.Wait()
}

func (c *Compiler) addSourceFile(ctx context.Context, cs *compileState, eg *errgroup.Group, p string, ref glfs.Ref) error {
	if gotIt := cs.claimPath(p); !gotIt {
		return cs.awaitPath(ctx, p)
	}
	defer cs.donePath(p)
	switch ref.Type {
	case glfs.TypeTree:
		tree, err := c.glfs.GetTree(ctx, c.store, ref)
		if err != nil {
			return err
		}
		for _, ent := range tree.Entries {
			ent := ent
			p2 := path.Join(p, ent.Name)
			eg.Go(func() error {
				return c.addSourceFile(ctx, cs, eg, p2, ent.Ref)
			})
		}

	case glfs.TypeBlob:
		ks := stringsets.Single(p)
		switch {
		case p == "WANT":
			// Drop the project configuration file from the build.
		case IsExprFilePath(p):
			return c.loadExpr(ctx, cs, p, ref)
		case IsStmtFilePath(p):
			return c.loadStmt(ctx, cs, p, ref)
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

func (c *Compiler) loadExpr(ctx context.Context, cs *compileState, p string, ref glfs.Ref) error {
	data, err := c.glfs.GetBlobBytes(ctx, c.store, ref, MaxJsonnetFileSize)
	if err != nil {
		return err
	}
	er, err := c.parseExprRoot(ctx, cs, p, data)
	if err != nil {
		return err
	}
	ks := er.Affects()
	vfs, rel := cs.acquireVFS()
	defer rel()
	if err := vfs.Add(VFSEntry{K: ks, V: er.expr, ConfigFile: p}); err != nil {
		panic(err)
	}
	cs.appendExprRoot(er)
	return nil
}

func (c *Compiler) loadStmt(ctx context.Context, cs *compileState, p string, ref glfs.Ref) error {
	data, err := c.glfs.GetBlobBytes(ctx, c.store, ref, MaxJsonnetFileSize)
	if err != nil {
		return err
	}
	ss, err := c.parseStmtSet(ctx, cs, p, data)
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
		if err := vfs.Add(VFSEntry{K: ks, V: stmt.expr(), ConfigFile: p, IsExport: isExport}); err != nil {
			rel()
			return fmt.Errorf("statement %d in file %q, outputs to conflicted keyspace %w", i, p, err)
		}
		rel()
	}
	cs.appendStmtSet(ss)
	return nil
}

// lowerSelections turns all selections in all the expressions into compute and fact
func (c *Compiler) lowerSelections(ctx context.Context, cs *compileState, p string) error {
	defer logStep(ctx, "lowering selections")()
	cache := make(map[[32]byte]Expr)
	vfs, rel := cs.acquireVFS()
	defer rel()
	for i, er := range cs.exprRoots {
		e2, err := c.replaceSelections(ctx, cache, vfs, er.expr)
		if err != nil {
			return err
		}
		cs.exprRoots[i].expr = e2
	}
	for _, ss := range cs.stmtSets {
		for _, stmt := range ss.stmts {
			e1 := stmt.expr()
			e2, err := c.replaceSelections(ctx, cache, vfs, e1)
			if err != nil {
				return err
			}
			stmt.setExpr(e2)
		}
	}
	root, err := c.replaceSelections(ctx, cache, vfs, cs.root)
	if err != nil {
		return err
	}
	cs.root = root
	return nil
}

func (c *Compiler) replaceSelections(ctx context.Context, cache map[[32]byte]Expr, vfs *VFS, expr Expr) (ret Expr, retErr error) {
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
			e2, err := c.replaceSelections(ctx, cache, vfs, input.From)
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
		e, err := c.query(ctx, vfs, x.set, x.pick)
		if err != nil {
			return nil, err
		}
		return c.replaceSelections(ctx, cache, vfs, e)
	case *value:
		return x, nil
	default:
		panic(x)
	}
}

func (c *Compiler) detectCycles(ctx context.Context, cs *compileState, p string) error {
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
	traverseExpr(cs.root)
	return check()
}

func (c *Compiler) makeGraph(ctx context.Context, cs *compileState, p string) error {
	defer logStep(ctx, "making graph")()
	gb := NewGraphBuilder(c.store)
	var targets []Target

	// ExprRoots
	for _, er := range cs.exprRoots {
		nid, err := gb.Expr(ctx, c.store, er.expr)
		if err != nil {
			return err
		}
		targets = append(targets, Target{
			To:        makePathSet(er.Affects()),
			From:      nid,
			DefinedIn: er.path,
		})
	}

	// StmtSets
	for _, ss := range cs.stmtSets {
		for _, stmt := range ss.stmts {
			nid, err := gb.Expr(ctx, c.store, stmt.expr())
			if err != nil {
				return err
			}
			_, isExport := stmt.(*exportStmt)
			targets = append(targets, Target{
				To:        makePathSet(stmt.Affects()),
				From:      nid,
				DefinedIn: ss.path,
				IsExport:  isExport,
			})
		}
	}

	root, err := gb.Expr(ctx, c.store, cs.root)
	if err != nil {
		return err
	}
	slices.SortFunc(targets, func(a, b Target) int {
		if a.DefinedIn != b.DefinedIn {
			return strings.Compare(a.DefinedIn, b.DefinedIn)
		}
		return strings.Compare(a.To.String(), b.To.String())
	})
	dag := gb.Finish()
	gref, err := wantdag.PostDAG(ctx, c.store, dag)
	if err != nil {
		return err
	}
	cs.graph = *gref
	cs.rootNode = root
	cs.targets = targets
	cs.nodeCount = uint64(gb.Count())
	return nil
}

func (c *Compiler) pickExpr(ctx context.Context, x Expr, p string) (Expr, error) {
	ref, err := c.glfs.PostBlob(ctx, c.store, strings.NewReader(p))
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

func (c *Compiler) filterExpr(ctx context.Context, x Expr, re *regexp.Regexp) (Expr, error) {
	ref, err := c.glfs.PostBlob(ctx, c.store, strings.NewReader(re.String()))
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
