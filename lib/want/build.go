package want

import (
	"context"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/glfstasks"
	"wantbuild.io/want/internal/op/dagops"
	"wantbuild.io/want/internal/op/wantops"
	"wantbuild.io/want/internal/stores"
	"wantbuild.io/want/internal/wantc"
	"wantbuild.io/want/internal/wantdag"
	"wantbuild.io/want/lib/wantjob"
	"wantbuild.io/want/lib/wantrepo"
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

type Target = wantc.Target

type TargetResult struct {
	ErrCode wantjob.ErrCode
	Data    []byte
	Ref     *glfs.Ref
}

func Build(ctx context.Context, jobs wantjob.System, buildCtx wantc.Metadata, src cadata.Getter, root glfs.Ref, prefix string) (*BuildResult, error) {
	// compile
	plan, planStore, err := doCompile(ctx, jobs, buildCtx, src, root)
	if err != nil {
		return nil, err
	}
	// execute build DAG
	dagRes, outStore, err := doGLFS(ctx, jobs,
		stores.Union{src, planStore},
		joinOpName("dag",
			dagops.OpExecAll), plan.DAG)
	if err != nil {
		return nil, err
	}
	// process results
	nrs, err := wantdag.GetNodeResults(ctx, outStore, *dagRes)
	if err != nil {
		return nil, err
	}
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
		Source:        root,
		OutputRoot:    outRoot,
		Targets:       plan.Targets,
		TargetResults: targetResults,

		Store: outStore,
	}, nil
}

func doCompile(ctx context.Context, jobs wantjob.System, buildCtx wantc.Metadata, srcStore cadata.Getter, srcRoot glfs.Ref) (*wantc.Plan, cadata.Getter, error) {
	scratch := stores.NewMem()
	ctRef, err := wantops.PostCompileTask(ctx, scratch, wantops.CompileTask{
		Ground:   srcRoot,
		Metadata: buildCtx,
	})
	if err != nil {
		return nil, nil, err
	}
	planRef, planStore, err := doGLFS(ctx, jobs,
		stores.Union{srcStore, scratch},
		joinOpName("want", wantops.OpCompile),
		*ctRef)
	if err != nil {
		return nil, nil, err
	}
	plan, err := wantc.GetPlan(ctx, planStore, *planRef)
	if err != nil {
		return nil, nil, err
	}
	return plan, planStore, nil
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
	return Build(ctx, sys.jobs, repo.Metadata(), srcStore, *srcRoot, prefix)
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
	plan, _, err := doCompile(ctx, sys.jobs, repo.Metadata(), srcStore, *srcRoot)
	if err != nil {
		return nil, err
	}
	return plan.Targets, nil
}
