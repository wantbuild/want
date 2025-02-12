// package wantops contains an executor for build system operations like compiling
package wantops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"
	"golang.org/x/sync/errgroup"

	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/internal/stringsets"
	"wantbuild.io/want/src/internal/wantc"
	"wantbuild.io/want/src/internal/wantdag"
	"wantbuild.io/want/src/wantcfg"
	"wantbuild.io/want/src/wantjob"
)

const (
	OpBuild             = wantjob.OpName("build")
	OpCompile           = wantjob.OpName("compile")
	OpCompileSnippet    = wantjob.OpName("compileSnippet")
	OpRegexpFromPathSet = wantjob.OpName("regexpFromPathSet")
)

const MaxSnippetSize = 1e7

var _ wantjob.Executor = &Executor{}

type Executor struct {
	CompileOp wantjob.OpName
	DAGExecOp wantjob.OpName
}

func (e Executor) Execute(jc wantjob.Ctx, src cadata.Getter, x wantjob.Task) ([]byte, error) {
	ctx := jc.Context
	switch x.Op {
	case OpBuild:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			return e.Build(jc, src, x)
		})
	case OpCompile:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			return e.Compile(jc, src, x)
		})
	case OpCompileSnippet:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			return e.CompileSnippet(ctx, jc.Dst, src, x)
		})
	case OpRegexpFromPathSet:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			return e.PathSetRegexp(jc, jc.Dst, src, x)
		})
	default:
		return nil, wantjob.NewErrUnknownOperator(x.Op)
	}
}

func (e Executor) Build(jc wantjob.Ctx, src cadata.Getter, x glfs.Ref) (*glfs.Ref, error) {
	defer jc.InfoSpan("build")()
	ctx := jc.Context
	buildTask, err := GetBuildTask(ctx, src, x)
	if err != nil {
		return nil, err
	}
	deps := make(map[wantc.ModuleID]glfs.Ref)
	nss := make(map[wantc.ModuleID]wantc.Namespace)
	if err := dependencyClosure(ctx, src, buildTask.Main, deps, nss, func(expr wantcfg.Expr) (*glfs.Ref, error) {
		return e.EvalExpr(jc, src, expr)
	}); err != nil {
		return nil, err
	}
	delete(deps, wantc.NewModuleID(buildTask.Main))
	jc.Infof("prepared %d dependencies", len(deps))
	// plan
	plan, planStore, err := DoCompile(ctx, jc.System, e.CompileOp, stores.Union{jc.Dst, src}, wantc.CompileTask{
		Module:    buildTask.Main,
		Metadata:  buildTask.Metadata,
		Namespace: nss[wantc.NewModuleID(buildTask.Main)],
		Deps:      deps,
	})
	if err != nil {
		return nil, err
	}
	// filter targets
	var targets []wantc.Target
	var dags []glfs.Ref
	for _, target := range plan.Targets {
		if wantc.Intersects(target.To, buildTask.Query) {
			targets = append(targets, target)
			dags = append(dags, target.DAG)
		}
	}
	// execute
	var errorsOccured bool
	results := make([]wantjob.Result, len(dags))
	src2 := stores.Union{src, jc.Dst, planStore}
	eg := errgroup.Group{}
	for i := range dags {
		i := i
		eg.Go(func() error {
			defer jc.InfoSpan("build " + targets[i].DefinedIn)()
			res, dagStore, err := wantjob.Do(ctx, jc.System, src2, wantjob.Task{
				Op:    e.DAGExecOp,
				Input: glfstasks.MarshalGLFSRef(dags[i]),
			})
			if err != nil {
				return err
			}
			errorsOccured = errorsOccured || res.ErrCode > 0
			if ref, err := glfstasks.ParseGLFSRef(res.Data); err == nil {
				if err := glfstasks.FastSync(ctx, jc.Dst, dagStore, *ref); err != nil {
					return err
				}
			}
			results[i] = *res
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}

	groundRef, err := wantc.Select(ctx, jc.Dst, src2, plan.Known, buildTask.Query)
	if err != nil {
		return nil, err
	}
	layers := []glfs.Ref{*groundRef}
	for i := range targets {
		if ref, err := glfstasks.ParseGLFSRef(results[i].Data); err == nil {
			layers = append(layers, *ref)
		}
	}
	outRef, err := glfs.Merge(ctx, jc.Dst, src2, layers...)
	if err != nil {
		return nil, err
	}
	if err := wantc.SyncPlan(ctx, jc.Dst, src2, *plan); err != nil {
		return nil, err
	}
	br, err := PostBuildResult(ctx, jc.Dst, BuildResult{
		Query:         buildTask.Query,
		Plan:          *plan,
		Targets:       targets,
		TargetResults: results,
		Output:        outRef,
	})
	if err != nil {
		return nil, err
	}
	if errorsOccured {
		err = fmt.Errorf("build failed")
	}
	return br, err
}

func (e Executor) Compile(jc wantjob.Ctx, s cadata.Getter, x glfs.Ref) (*glfs.Ref, error) {
	defer jc.InfoSpan("compile")()
	ctx := jc.Context
	dst := jc.Dst
	ct, err := GetCompileTask(ctx, s, x)
	if err != nil {
		return nil, err
	}
	c := wantc.NewCompiler()
	plan, err := c.Compile(ctx, dst, s, *ct)
	if err != nil {
		return nil, err
	}
	return wantc.PostPlan(ctx, dst, *plan)
}

func (e Executor) CompileSnippet(ctx context.Context, dst cadata.Store, s cadata.Getter, ref glfs.Ref) (*glfs.Ref, error) {
	c := wantc.NewCompiler()
	data, err := glfs.GetBlobBytes(ctx, s, ref, MaxSnippetSize)
	if err != nil {
		return nil, err
	}
	dag, err := c.CompileSnippet(ctx, dst, s, data)
	if err != nil {
		return nil, err
	}
	return wantdag.PostDAG(ctx, dst, dag)
}

func (e Executor) PathSetRegexp(jc wantjob.Ctx, dst cadata.Store, s cadata.Getter, ref glfs.Ref) (*glfs.Ref, error) {
	ctx := jc.Context
	data, err := glfs.GetBlobBytes(ctx, s, ref, 1e6)
	if err != nil {
		return nil, err
	}
	var pathSet wantcfg.PathSet
	if err := json.Unmarshal(data, &pathSet); err != nil {
		return nil, err
	}
	set := wantc.SetFromQuery("", pathSet)
	stringsets.Simplify(set)
	jc.Infof("set: %v", set)
	re := set.Regexp()
	jc.Infof("re: %v", re)
	return glfs.PostBlob(ctx, dst, strings.NewReader(re.String()))
}

func DoCompile(ctx context.Context, sys wantjob.System, compileOp wantjob.OpName, src cadata.Getter, ct wantc.CompileTask) (*wantc.Plan, cadata.Getter, error) {
	scratch := stores.NewMem()
	for _, ref := range ct.Namespace {
		if err := glfs.Sync(ctx, scratch, src, ref); err != nil {
			return nil, nil, err
		}
	}
	ctRef, err := PostCompileTask(ctx, stores.Fork{W: scratch, R: src}, ct)
	if err != nil {
		return nil, nil, err
	}
	planRef, planStore, err := glfstasks.Do(ctx, sys, stores.Union{src, scratch}, compileOp, *ctRef)
	if err != nil {
		return nil, nil, err
	}
	plan, err := wantc.GetPlan(ctx, planStore, *planRef)
	if err != nil {
		return nil, nil, err
	}
	return plan, planStore, nil
}

func (e Executor) EvalExpr(jc wantjob.Ctx, src cadata.Getter, expr wantcfg.Expr) (*glfs.Ref, error) {
	ctx := jc.Context
	c := wantc.NewCompiler()
	dag, err := c.CompileExpr(ctx, jc.Dst, src, expr)
	if err != nil {
		return nil, err
	}
	dagRef, err := wantdag.PostDAG(ctx, jc.Dst, dag)
	if err != nil {
		return nil, err
	}
	outRef, outGet, err := glfstasks.Do(ctx, jc.System, jc.Dst, e.DAGExecOp, *dagRef)
	if err != nil {
		return nil, err
	}
	if err := glfstasks.FastSync(ctx, jc.Dst, outGet, *outRef); err != nil {
		return nil, err
	}
	return outRef, nil
}

func dependencyClosure(ctx context.Context, src cadata.Getter, modRef glfs.Ref, deps map[wantc.ModuleID]glfs.Ref, nss map[wantc.ModuleID]wantc.Namespace, eval func(wantcfg.Expr) (*glfs.Ref, error)) error {
	modid := wantc.NewModuleID(modRef)
	if _, exists := deps[modid]; exists {
		return nil
	}
	if modRef.Type == glfs.TypeTree {
		modCfg, err := wantc.GetModuleConfig(ctx, src, modRef)
		if err != nil {
			return err
		}
		nss[modid] = make(wantc.Namespace)
		for name, expr := range modCfg.Namespace {
			ref, err := eval(expr)
			if err != nil {
				return err
			}
			if err := dependencyClosure(ctx, src, *ref, deps, nss, eval); err != nil {
				return err
			}
			ns := nss[modRef.CID]
			ns[name] = *ref
		}
	}
	deps[modid] = modRef
	return nil
}
