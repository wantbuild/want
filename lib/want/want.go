package want

import (
	"context"
	"runtime"

	"github.com/blobcache/glfs"
	"github.com/jmoiron/sqlx"

	"wantbuild.io/want/internal/op/dagops"
	"wantbuild.io/want/internal/stores"
	"wantbuild.io/want/internal/wantc"
	"wantbuild.io/want/internal/wantdag"
	"wantbuild.io/want/internal/wantjob"
	"wantbuild.io/want/lib/wantrepo"
)

func Build(ctx context.Context, db *sqlx.DB, repo *wantrepo.Repo, prefix string) (*glfs.Ref, error) {
	panic("not implemented")
}

func Eval(ctx context.Context, db *sqlx.DB, repo *wantrepo.Repo, calledFrom string, expr []byte) (*glfs.Ref, error) {
	s := stores.NewMem()
	exec := newExecutor(s)
	jsys := newJobSys(ctx, db, exec, s, runtime.GOMAXPROCS(0))
	defer jsys.Shutdown()

	c := wantc.NewCompiler(s)
	dag, err := c.CompileSnippet(ctx, expr)
	if err != nil {
		return nil, err
	}
	dagRef, err := wantdag.PostDAG(ctx, s, *dag)
	if err != nil {
		return nil, err
	}
	task := wantjob.Task{
		Op:    "dag." + dagops.OpExecLast,
		Input: *dagRef,
	}

	rootIdx, err := jsys.Init(ctx, task)
	if err != nil {
		return nil, err
	}
	if err := jsys.Await(ctx, nil, rootIdx); err != nil {
		return nil, err
	}
	job, err := jsys.Inspect(ctx, nil, rootIdx)
	if err != nil {
		return nil, err
	}
	if err := job.Result.Err(); err != nil {
		return nil, err
	} else {
		return &job.Result.Data, nil
	}
}
