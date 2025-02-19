package want

import (
	"context"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/internal/op/dagops"
	"wantbuild.io/want/src/internal/op/wantops"
	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/internal/wantc"
	"wantbuild.io/want/src/internal/wantdag"
	"wantbuild.io/want/src/internal/wantrepo"
	"wantbuild.io/want/src/wantcfg"
	"wantbuild.io/want/src/wantjob"
)

type (
	BuildTask = wantops.BuildTask
	Target    = wantc.Target
)

// BuildResult is the output of a build
type BuildResult struct {
	Source        glfs.Ref
	Targets       []Target
	TargetResults []wantjob.Result
	OutputRoot    *glfs.Ref

	// TODO: remove
	Store cadata.Getter
}

func Build(ctx context.Context, jobs wantjob.System, src cadata.Getter, bt BuildTask) (*BuildResult, error) {
	scratch := stores.NewMem()
	btRef, err := wantops.PostBuildTask(ctx, stores.Fork{W: scratch, R: src}, bt)
	if err != nil {
		return nil, err
	}
	outRef, outStore, err := glfstasks.Do(ctx, jobs, stores.Union{src, scratch}, joinOpName("want", wantops.OpBuild), *btRef)
	if err != nil {
		return nil, err
	}
	br, err := wantops.GetBuildResult(ctx, outStore, *outRef)
	if err != nil {
		return nil, err
	}
	return &BuildResult{
		Source:        bt.Main,
		Targets:       br.Targets,
		TargetResults: br.TargetResults,
		OutputRoot:    br.Output,

		Store: outStore,
	}, nil
}

func (sys *System) Build(ctx context.Context, repo *wantrepo.Repo, query wantcfg.PathSet) (*BuildResult, error) {
	afid, err := sys.Import(ctx, repo)
	if err != nil {
		return nil, err
	}
	af, err := sys.ViewArtifact(ctx, *afid)
	if err != nil {
		return nil, err
	}
	root, err := af.GLFS()
	if err != nil {
		return nil, err
	}
	return Build(ctx, sys.jobs, af.Store, BuildTask{
		Main:     *root,
		Metadata: repo.Metadata(),
		Query:    query,
	})
}

// Blame lists the build targets
func (sys *System) Blame(ctx context.Context, repo *wantrepo.Repo) ([]Target, error) {
	afid, err := sys.Import(ctx, repo)
	if err != nil {
		return nil, err
	}
	af, err := sys.ViewArtifact(ctx, *afid)
	if err != nil {
		return nil, err
	}
	root, err := af.GLFS()
	if err != nil {
		return nil, err
	}
	jctx := wantjob.Ctx{Context: ctx, Dst: stores.NewMem(), System: sys.jobs}
	deps, err := wantops.MakeDeps(jctx, af.Store, *root, func(x wantcfg.Expr) (*glfs.Ref, error) {
		ref, store, err := sys.evalExpr(ctx, x)
		if err != nil {
			return nil, err
		}
		if err := glfstasks.FastSync(ctx, jctx.Dst, store, *ref); err != nil {
			return nil, err
		}
		return ref, nil
	})
	if err != nil {
		return nil, err
	}
	plan, _, err := wantops.DoCompile(ctx, sys.jobs, joinOpName("want", wantops.OpCompile), af.Store, wantc.CompileTask{
		Module:   *root,
		Metadata: repo.Metadata(),
		Deps:     deps,
	})
	if err != nil {
		return nil, err
	}
	return plan.Targets, nil
}

func (sys *System) evalExpr(ctx context.Context, x wantcfg.Expr) (*glfs.Ref, cadata.Getter, error) {
	s := stores.NewMem()
	c := wantc.NewCompiler()
	dag, err := c.CompileExpr(ctx, s, s, x)
	if err != nil {
		return nil, nil, err
	}
	dagRef, err := wantdag.PostDAG(ctx, s, dag)
	if err != nil {
		return nil, nil, err
	}
	return glfstasks.Do(ctx, sys.jobs, s, joinOpName("dag", dagops.OpExecLast), *dagRef)
}
