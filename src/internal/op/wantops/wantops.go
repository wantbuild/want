// package wantops contains an executor for build system operations like compiling
package wantops

import (
	"context"
	"errors"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/internal/wantc"
	"wantbuild.io/want/src/internal/wantdag"
	"wantbuild.io/want/src/wantcfg"
	"wantbuild.io/want/src/wantjob"
)

const (
	OpBuild          = wantjob.OpName("build")
	OpCompile        = wantjob.OpName("compile")
	OpCompileSnippet = wantjob.OpName("compileSnippet")
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
			buildTask, err := GetBuildTask(ctx, src, x)
			if err != nil {
				return nil, err
			}
			defer jc.InfoSpan("build")()
			ctx := jc.Context

			br, err := e.Build(jc, src, *buildTask)
			if err != nil {
				return nil, err
			}
			outRef, err := PostBuildResult(ctx, jc.Dst, *br)
			if err != nil {
				return nil, err
			}
			if br.ErrorCount() > 0 {
				err = errors.New("errors occured")
			}
			return outRef, err
		})
	case OpCompile:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			return e.Compile(jc, src, x)
		})
	case OpCompileSnippet:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			return e.CompileSnippet(ctx, jc.Dst, src, x)
		})
	default:
		return nil, wantjob.NewErrUnknownOperator(x.Op)
	}
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

func DoCompile(ctx context.Context, sys wantjob.System, compileOp wantjob.OpName, src cadata.Getter, ct wantc.CompileTask) (*wantc.Plan, cadata.Getter, error) {
	scratch := stores.NewMem()
	for _, ref := range ct.Deps {
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

func dependencyClosure(ctx context.Context, src cadata.Getter, modRef glfs.Ref, deps map[wantc.ExprID]glfs.Ref, eval func(wantcfg.Expr) (*glfs.Ref, error)) error {
	if modRef.Type == glfs.TypeTree {
		modCfg, err := wantc.GetModuleConfig(ctx, src, modRef)
		if err != nil {
			return err
		}
		for _, expr := range modCfg.Namespace {
			eid := wantc.NewExprID(expr)
			deps[eid] = modRef
			ref, err := eval(expr)
			if err != nil {
				return err
			}
			if err := dependencyClosure(ctx, src, *ref, deps, eval); err != nil {
				return err
			}
			deps[eid] = *ref
		}
	}
	return nil
}
