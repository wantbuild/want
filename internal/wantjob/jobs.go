package wantjob

import (
	"context"
	"fmt"
	"iter"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"
)

type Job struct {
	Task    Task
	Output  glfs.Ref
	Error   error
	StartAt time.Time
	EndAt   *time.Time
}

func (j Job) Elapsed() time.Duration {
	if j.EndAt == nil {
		return time.Since(j.StartAt)
	}
	return j.EndAt.Sub(j.StartAt)
}

type JobID []string

func (jid JobID) String() string {
	return strings.Join(jid, "/")
}

// JobCtx is available inside a running job
type JobCtx struct {
	exec Executor
	s    cadata.Store

	cf       context.CancelFunc
	done     chan struct{}
	job      *Job
	children []*JobCtx
}

func Run(ctx context.Context, exec Executor, src cadata.Store, task Task) (*Job, error) {
	rootCtx := JobCtx{
		exec: exec,
		s:    src,
	}
	out, err := exec.Compute(ctx, &rootCtx, src, task)
	if err != nil {
		return nil, err
	}
	return &Job{
		Task:   task,
		Output: *out,
	}, nil
}

func (jc *JobCtx) Spawn(ctx context.Context, task Task) (string, error) {
	n := len(jc.children)
	name := strconv.FormatUint(uint64(n), 16)

	ctx2, cf := context.WithCancel(ctx)
	childCtx := JobCtx{
		exec: jc.exec,
		s:    jc.s,

		cf:   cf,
		done: make(chan struct{}),
		job: &Job{
			Task:    task,
			StartAt: time.Now(),
		},
	}
	jc.children = append(jc.children, &childCtx)
	go func() {
		defer cf()
		defer close(childCtx.done)
		j := childCtx.job
		out, err := jc.exec.Compute(ctx2, &childCtx, childCtx.s, task)
		if err != nil {
			j.Error = err
		} else {
			j.Output = *out
		}
	}()

	return name, nil
}

func (jc *JobCtx) Await(ctx context.Context, name string) error {
	n, err := strconv.ParseUint(name, 16, 64)
	if err != nil {
		return err
	}
	if len(jc.children) <= int(n) {
		return fmt.Errorf("job does not exist: %v", name)
	}
	childCtx := jc.children[n]
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-childCtx.done:
		return nil
	}
}

func (jc *JobCtx) Cancel(ctx context.Context, name string) error {
	return nil
}

func (jc *JobCtx) Inspect(ctx context.Context, name string) (*Job, error) {
	jc, err := jc.getChild(name)
	if err != nil {
		return nil, err
	}
	return jc.job, nil
}

func (jc *JobCtx) getChild(name string) (*JobCtx, error) {
	n, err := strconv.ParseUint(name, 16, 64)
	if err != nil {
		return nil, err
	}
	if len(jc.children) <= int(n) {
		return nil, fmt.Errorf("job does not exist: %v", name)
	}
	return jc.children[n], nil
}

func (jc *JobCtx) Children(ctx context.Context) iter.Seq[string] {
	return func(yield func(string) bool) {
		for i := range jc.children {
			jobid := strconv.FormatUint(uint64(i), 16)
			if !yield(jobid) {
				break
			}
		}
	}
}

func (jc *JobCtx) Infof(msg string, args ...any) {
	fmt.Fprintf(os.Stderr, msg+"\n", args)
}

func (jc *JobCtx) Debugf(msg string, args ...any) {
	fmt.Fprintf(os.Stderr, msg+"\n", args)
}

// Do spawns a child job to compute the Task, then awaits it and returns the result
func Do(ctx context.Context, jc *JobCtx, task Task) (*glfs.Ref, error) {
	id, err := jc.Spawn(ctx, task)
	if err != nil {
		return nil, err
	}
	if err := jc.Await(ctx, id); err != nil {
		return nil, err
	}
	job, err := jc.Inspect(ctx, id)
	if err != nil {
		return nil, err
	}
	if job.Error != nil {
		return nil, job.Error
	} else {
		return &job.Output, nil
	}
}
