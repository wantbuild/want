package want

import (
	"context"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/glfstasks"
	"wantbuild.io/want/internal/op/wantops"
	"wantbuild.io/want/internal/stores"
	"wantbuild.io/want/internal/wantc"
	"wantbuild.io/want/lib/wantjob"
	"wantbuild.io/want/lib/wantrepo"
)

type (
	BuildTask = wantops.BuildTask
	Target    = wantc.Target
)

// BuildResult is the output of a build
type BuildResult struct {
	Source        glfs.Ref
	Targets       []Target
	TargetResults []TargetResult
	OutputRoot    *glfs.Ref

	// TODO: remove
	Store cadata.Getter
}

type TargetResult struct {
	ErrCode wantjob.ErrCode
	Data    []byte
	Ref     *glfs.Ref
}

func Build(ctx context.Context, jobs wantjob.System, src cadata.Getter, bt BuildTask) (*BuildResult, error) {
	scratch := stores.NewMem()

	btRef, err := wantops.PostBuildTask(ctx, scratch, bt)
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

	nrs := br.NodeResults
	plan := br.Plan
	rootRes := nrs[plan.LastNode]
	outRoot, err := glfstasks.ParseGLFSRef(rootRes.Data)
	if err != nil {
		return nil, err
	}
	targetResults := make([]TargetResult, len(plan.Targets))
	for i := range targetResults {
		targ := plan.Targets[i]
		res := nrs[targ.Node]
		ref, _ := glfstasks.ParseGLFSRef(res.Data)
		targetResults[i] = TargetResult{
			ErrCode: res.ErrCode,
			Ref:     ref,
		}
	}
	return &BuildResult{
		Source:        bt.Main,
		OutputRoot:    outRoot,
		Targets:       plan.Targets,
		TargetResults: targetResults,

		Store: outStore,
	}, nil
}

func (sys *System) Build(ctx context.Context, repo *wantrepo.Repo, prefix string) (*BuildResult, error) {
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
		Prefix:   prefix,
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
	plan, _, err := wantops.DoCompile(ctx, sys.jobs, joinOpName("want", wantops.OpCompile), srcStore, wantops.CompileTask{
		Module:   *srcRoot,
		Metadata: repo.Metadata(),
	})
	if err != nil {
		return nil, err
	}
	return plan.Targets, nil
}
