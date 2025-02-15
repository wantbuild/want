package wantops

import (
	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"
	"golang.org/x/sync/errgroup"

	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/internal/wantc"
	"wantbuild.io/want/src/wantcfg"
	"wantbuild.io/want/src/wantjob"
)

func (e *Executor) Build(jc wantjob.Ctx, src cadata.Getter, buildTask BuildTask) (*BuildResult, error) {
	ctx := jc.Context
	deps := make(map[wantc.ExprID]glfs.Ref)
	eval := func(expr wantcfg.Expr) (*glfs.Ref, error) {
		return e.EvalExpr(jc, src, expr)
	}
	if err := dependencyClosure(ctx, src, buildTask.Main, deps, eval); err != nil {
		return nil, err
	}
	jc.Infof("prepared %d dependencies", len(deps))
	// plan
	plan, planStore, err := DoCompile(ctx, jc.System, e.CompileOp, stores.Union{jc.Dst, src}, wantc.CompileTask{
		Module:   buildTask.Main,
		Metadata: buildTask.Metadata,
		Deps:     deps,
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
	return &BuildResult{
		Query:         buildTask.Query,
		Plan:          *plan,
		Targets:       targets,
		TargetResults: results,
		Output:        outRef,
	}, nil
}
