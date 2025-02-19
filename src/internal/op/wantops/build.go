package wantops

import (
	"context"
	"fmt"
	"strings"

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
	deps, err := MakeDeps(jc, src, buildTask.Main, func(expr wantcfg.Expr) (*glfs.Ref, error) {
		jc2 := jc
		jc2.System = maskJobs{jc2.System}
		return e.EvalExpr(jc2, src, expr)
	})
	if err != nil {
		return nil, fmt.Errorf("preparing dependencies: %w", err)
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
	if err := func() error {
		eg, ctx := errgroup.WithContext(jc.Context)
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
		return eg.Wait()
	}(); err != nil {
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

func MakeDeps(jc wantjob.Ctx, src cadata.Getter, modRef glfs.Ref, eval func(wantcfg.Expr) (*glfs.Ref, error)) (map[wantc.ExprID]glfs.Ref, error) {
	deps := make(map[wantc.ExprID]glfs.Ref)
	if err := dependencyClosure(jc.Context, stores.Union{jc.Dst, src}, modRef, deps, eval); err != nil {
		return nil, err
	}
	return deps, nil
}

var _ wantjob.System = maskJobs{}

type maskJobs struct {
	wantjob.System
}

func (mj maskJobs) Spawn(ctx context.Context, src cadata.Getter, task wantjob.Task) (wantjob.Idx, error) {
	x := string(task.Op)
	allowed := []string{
		"dag.",
		"want.",
		"import.",
		"glfs.",
		"assert.",
	}
	for _, prefix := range allowed {
		if strings.HasPrefix(x, prefix) {
			return mj.System.Spawn(ctx, src, task)
		}
	}
	return 0, fmt.Errorf("cannot use op %v in pre-compile step", task.Op)
}
