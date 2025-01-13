package want

import (
	"context"
	"strings"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/op/dagops"
	"wantbuild.io/want/internal/op/glfsops"
	"wantbuild.io/want/internal/op/wantops"
	"wantbuild.io/want/internal/singleflight"
	"wantbuild.io/want/internal/wantjob"
)

type executor struct {
	s     cadata.Store
	execs map[wantjob.OpName]wantjob.Executor
	sf    singleflight.Group[wantjob.Task, *glfs.Ref]
}

func newExecutor(s cadata.Store) *executor {
	glfsExec := glfsops.NewExecutor(s)
	wantExec := wantops.NewExecutor(s)
	dagExec := dagops.NewExecutor(s)
	return &executor{
		s: s,
		execs: map[wantjob.OpName]wantjob.Executor{
			"glfs": glfsExec,
			"want": wantExec,
			"dag":  dagExec,
		},
	}
}

func (e *executor) Compute(ctx context.Context, jc *wantjob.JobCtx, src cadata.Getter, task wantjob.Task) (*glfs.Ref, error) {
	parts := strings.SplitN(string(task.Op), ".", 2)
	e2, exists := e.execs[wantjob.OpName(parts[0])]
	if !exists {
		return nil, wantjob.ErrOpNotFound{Op: task.Op}
	}
	out, err, _ := e.sf.Do(task, func() (*glfs.Ref, error) {
		return e2.Compute(ctx, jc, src, wantjob.Task{
			Op:    wantjob.OpName(parts[1]),
			Input: task.Input,
		})
	})
	return out, err
}

func (e *executor) GetStore() cadata.Getter {
	return e.s
}
