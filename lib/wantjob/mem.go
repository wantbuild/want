package wantjob

import (
	"context"
	"fmt"
	"sync"

	"go.brendoncarroll.net/state/cadata"
	"wantbuild.io/want/internal/stores"
)

type memJob struct {
	exec Executor
	src  cadata.Getter
	task Task

	ctx      context.Context
	cf       context.CancelFunc
	dst      cadata.Store
	children []*memJob

	doneOnce sync.Once
	done     chan struct{}
	res      *Result
}

func newMemJob(parentCtx context.Context, exec Executor, src cadata.Getter, task Task) *memJob {
	ctx2, cf := context.WithCancel(parentCtx)
	return &memJob{
		exec: exec,
		src:  src,
		task: task,

		ctx: ctx2, cf: cf,
		dst:  stores.NewMem(),
		done: make(chan struct{}),
	}
}

func (j *memJob) Spawn(ctx context.Context, src cadata.Getter, task Task) (Idx, error) {
	n := len(j.children)
	child := newMemJob(j.ctx, j.exec, src, task)
	j.children = append(j.children, child)

	go func() {
		jc := Ctx{
			Context: j.ctx,
			Dst:     child.dst,
			System:  child,
		}
		out, err := j.exec.Execute(jc, src, task)
		if err != nil {
			child.res = Result_ErrExec(err)
		} else {
			child.res = Success(out)
		}
		child.doneOnce.Do(func() {
			close(child.done)
		})
	}()
	return Idx(n), nil
}

func (j *memJob) Cancel(ctx context.Context, idx Idx) error {
	child := j.children[idx]

	child.cf()
	child.doneOnce.Do(func() {
		close(child.done)
	})
	return nil
}

func (j *memJob) Await(ctx context.Context, idx Idx) error {
	done := j.children[idx].done
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (j *memJob) Inspect(ctx context.Context, idx Idx) (*Job, error) {
	panic("Inspect not implemented")
}

func (j *memJob) ViewResult(ctx context.Context, idx Idx) (*Result, cadata.Getter, error) {
	child := j.children[idx]
	if !child.isDone() {
		return nil, nil, fmt.Errorf("ViewResult called on unfinished Job")
	}
	return child.res, child.dst, nil
}

func (j *memJob) isDone() bool {
	select {
	case <-j.done:
		return true
	default:
		return false
	}
}

func NewMem(bgCtx context.Context, exec Executor) System {
	return newMemJob(bgCtx, exec, nil, Task{})
}
