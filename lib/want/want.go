package want

import (
	"context"
	"runtime"

	"github.com/blobcache/glfs"
	"github.com/jmoiron/sqlx"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/glfstasks"
	"wantbuild.io/want/internal/op/dagops"
	"wantbuild.io/want/internal/stores"
	"wantbuild.io/want/internal/wantc"
	"wantbuild.io/want/internal/wantdag"
	"wantbuild.io/want/lib/wantjob"
	"wantbuild.io/want/lib/wantrepo"
)

func Eval(ctx context.Context, db *sqlx.DB, repo *wantrepo.Repo, calledFrom string, expr []byte) (*glfs.Ref, cadata.Getter, error) {
	s := stores.NewMem()
	exec := newExecutor()
	jsys := newJobSys(ctx, db, exec, runtime.GOMAXPROCS(0))
	defer jsys.Shutdown()

	c := wantc.NewCompiler()
	dag, err := c.CompileSnippet(ctx, s, s, expr)
	if err != nil {
		return nil, nil, err
	}
	dagRef, err := wantdag.PostDAG(ctx, s, *dag)
	if err != nil {
		return nil, nil, err
	}
	task := wantjob.Task{
		Op:    joinOpName("dag", dagops.OpExecLast),
		Input: glfstasks.MarshalGLFSRef(*dagRef),
	}
	return runRootJob(ctx, jsys, s, task)
}

func runRootJob(ctx context.Context, jsys *JobSys, src cadata.Getter, task wantjob.Task) (*glfs.Ref, cadata.Getter, error) {
	rootIdx, err := jsys.Init(ctx, src, task)
	if err != nil {
		return nil, nil, err
	}
	if err := jsys.Await(ctx, nil, rootIdx); err != nil {
		return nil, nil, err
	}
	job, err := jsys.Inspect(ctx, nil, rootIdx)
	if err != nil {
		return nil, nil, err
	}
	if err := job.Result.Err(); err != nil {
		return nil, nil, err
	} else {
		ref, err := glfstasks.ParseGLFSRef(job.Result.Data)
		rootState := jsys.getJobState(wantjob.JobID{rootIdx})
		return ref, rootState.dst, err
	}
}

func joinOpName(xs ...dagops.OpName) (ret dagops.OpName) {
	for i, x := range xs {
		if i > 0 {
			ret += "."
		}
		ret += x
	}
	return ret
}
