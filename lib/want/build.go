package want

import (
	"context"
	"runtime"

	"github.com/blobcache/glfs"
	"github.com/jmoiron/sqlx"
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

func Blame(ctx context.Context, db *sqlx.DB, repo *wantrepo.Repo) ([]Target, error) {
	srcid, err := Import(ctx, db, repo)
	if err != nil {
		return nil, err
	}
	srcRoot, srcStore, err := AccessSource(ctx, db, srcid)
	if err != nil {
		return nil, err
	}

	cstore := stores.NewMem()
	c := wantc.NewCompiler()
	plan, err := c.Compile(ctx, cstore, srcStore, repo.Metadata(), *srcRoot)
	if err != nil {
		return nil, err
	}
	return plan.Targets, nil
}

func Build(ctx context.Context, db *sqlx.DB, repo *wantrepo.Repo, prefix string) (*BuildResult, error) {
	srcid, err := Import(ctx, db, repo)
	if err != nil {
		return nil, err
	}
	srcRoot, srcStore, err := AccessSource(ctx, db, srcid)
	if err != nil {
		return nil, err
	}
	exec := newExecutor()
	jsys := newJobSys(ctx, db, exec, runtime.GOMAXPROCS(0))
	defer jsys.Shutdown()

	// compile
	plan, planStore, err := compile(ctx, jsys, repo.Metadata(), srcStore, *srcRoot)
	if err != nil {
		return nil, err
	}
	// execute build DAG
	dagRes, outStore, err := runRootJob(ctx, jsys, stores.Union{srcStore, planStore}, wantjob.Task{
		Op:    joinOpName("dag", dagops.OpExecAll),
		Input: glfstasks.MarshalGLFSRef(plan.DAG),
	})
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
		targetResults[i] = TargetResult{
			ErrCode: res.ErrCode,
		}
	}
	return &BuildResult{
		OutputRoot:    outRoot,
		Targets:       plan.Targets,
		TargetResults: targetResults,

		Store: outStore,
	}, nil
}

func compile(ctx context.Context, jsys *JobSys, buildCtx wantc.Metadata, srcStore cadata.Getter, srcRoot glfs.Ref) (*wantc.Plan, cadata.Getter, error) {
	scratch := stores.NewMem()
	ctRef, err := wantops.PostCompileTask(ctx, scratch, wantops.CompileTask{
		Ground:   srcRoot,
		Metadata: buildCtx,
	})
	if err != nil {
		return nil, nil, err
	}
	planRef, planStore, err := runRootJob(ctx, jsys, stores.Union{srcStore, scratch}, wantjob.Task{
		Op:    joinOpName("want", wantops.OpCompile),
		Input: glfstasks.MarshalGLFSRef(*ctRef),
	})
	if err != nil {
		return nil, nil, err
	}
	plan, err := wantc.GetPlan(ctx, planStore, *planRef)
	if err != nil {
		return nil, nil, err
	}
	return plan, planStore, nil
}
