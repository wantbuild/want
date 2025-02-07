package want

import (
	"context"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/internal/op/wantops"
	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/internal/wantc"
	"wantbuild.io/want/src/wantcfg"
	"wantbuild.io/want/src/wantjob"
	"wantbuild.io/want/src/wantrepo"
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
	srcid, err := sys.Import(ctx, repo)
	if err != nil {
		return nil, err
	}
	srcRoot, srcStore, err := sys.AccessSource(ctx, srcid)
	if err != nil {
		return nil, err
	}
	return Build(ctx, sys.jobs, srcStore, BuildTask{
		Main:     *srcRoot,
		Metadata: repo.Metadata(),
		Query:    query,
	})
}

// Blame lists the build targets
func (sys *System) Blame(ctx context.Context, repo *wantrepo.Repo) ([]Target, error) {
	srcid, err := sys.Import(ctx, repo)
	if err != nil {
		return nil, err
	}
	srcRoot, srcStore, err := sys.AccessSource(ctx, srcid)
	if err != nil {
		return nil, err
	}
	plan, _, err := wantops.DoCompile(ctx, sys.jobs, joinOpName("want", wantops.OpCompile), srcStore, wantc.CompileTask{
		Module:   *srcRoot,
		Metadata: repo.Metadata(),
	})
	if err != nil {
		return nil, err
	}
	return plan.Targets, nil
}
