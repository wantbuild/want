package want

import (
	"context"

	"github.com/blobcache/glfs"
	"github.com/jmoiron/sqlx"

	"wantbuild.io/want/internal/dbutil"
	"wantbuild.io/want/internal/op/dagops"
	"wantbuild.io/want/internal/stores"
	"wantbuild.io/want/internal/wantc"
	"wantbuild.io/want/internal/wantdag"
	"wantbuild.io/want/internal/wantjob"
	"wantbuild.io/want/lib/wantrepo"
)

func Plan(ctx context.Context, db *sqlx.DB, root glfs.Ref) (*wantc.Plan, error) {
	panic("not implemented")
}

func Build(ctx context.Context, db *sqlx.DB, plan *wantc.Plan) (*glfs.Ref, error) {
	panic("not implemented")
}

func Eval(ctx context.Context, db *sqlx.DB, repo *wantrepo.Repo, calledFrom string, expr []byte) (*glfs.Ref, error) {
	return dbutil.DoTx1(ctx, db, func(tx *sqlx.Tx) (*glfs.Ref, error) {
		s := stores.NewMem()
		c := wantc.NewCompiler(s)
		dag, err := c.CompileSnippet(ctx, expr)
		if err != nil {
			return nil, err
		}
		dagRef, err := wantdag.PostDAG(ctx, s, *dag)
		if err != nil {
			return nil, err
		}
		exec := newExecutor(s)

		job, err := wantjob.Run(ctx, exec, s, wantjob.Task{
			Op:    "dag." + dagops.OpExecLast,
			Input: *dagRef,
		})
		if err != nil {
			return nil, err
		}
		if job.Error != nil {
			return nil, job.Error
		} else {
			return &job.Output, nil
		}
	})
}
