package wantjob

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.brendoncarroll.net/exp/slices2"
	"go.brendoncarroll.net/state/cadata"
	"go.brendoncarroll.net/tai64"
)

// Idx is a component of a job id.
// It uniquely identifies a job within its parent.
// It is the number of sibling jobs created before it.
type Idx uint32

func (idx Idx) String() string {
	return fmt.Sprintf("%08x", uint32(idx))
}

type JobID []Idx

func (jid JobID) String() string {
	return strings.Join(slices2.Map(jid, func(x Idx) string {
		return x.String()
	}), "/")
}

type JobState uint32

const (
	JobState_UNKNOWN = iota
	QUEUED
	RUNNING
	DONE
)

type ErrCode uint32

const (
	// OK means the job completed successfully
	OK = iota
	// TIMEOUT means the system lost contact with the job or it was taking too long
	TIMEOUT
	// CANCELLED means the job was cancelled before it could complete
	CANCELLED
	// EXEC_FAILURE is an execution error
	EXEC_FAILURE
)

// Result is produced by finished jobs.
// Jobs will also have results when cancelled or timed out, with the situation reflected in the ErrCode
type Result struct {
	ErrCode ErrCode `json:"ec"`
	Data    []byte  `json:"d"`
}

func Success(data []byte) *Result {
	return &Result{Data: data}
}

func Result_ErrExec(err error) *Result {
	return &Result{ErrCode: EXEC_FAILURE, Data: []byte(err.Error())}
}

func (r *Result) Err() error {
	if r.ErrCode == 0 {
		return nil
	}
	return fmt.Errorf("job failed errcode=%v data=%q", r.ErrCode, r.Data)
}

type Job struct {
	Task    Task
	State   JobState
	StartAt tai64.TAI64N

	Result *Result
	EndAt  *tai64.TAI64N
}

func (j Job) Elapsed() time.Duration {
	if j.EndAt == nil {
		return time.Since(j.StartAt.GoTime())
	}
	return j.EndAt.GoTime().Sub(j.StartAt.GoTime())
}

// System manages spawning, running, and awaiting jobs.
type System interface {
	Spawn(ctx context.Context, parent JobID, src cadata.Getter, task Task) (Idx, error)
	Cancel(ctx context.Context, parent JobID, idx Idx) error
	Await(ctx context.Context, parent JobID, idx Idx) error
	Inspect(ctx context.Context, parent JobID, idx Idx) (*Job, error)
}

// Ctx is a Job Context.  It is the API available from within a running job
type Ctx struct {
	ctx context.Context
	sys System
	id  JobID
}

func NewCtx(ctx context.Context, sys System, id JobID) Ctx {
	return Ctx{sys: sys, id: id, ctx: ctx}
}

func (jc *Ctx) Context() context.Context {
	return jc.ctx
}

func (jc *Ctx) Spawn(ctx context.Context, src cadata.Getter, task Task) (Idx, error) {
	return jc.sys.Spawn(ctx, jc.id, src, task)
}

func (jc *Ctx) Await(ctx context.Context, idx Idx) error {
	return jc.sys.Await(ctx, jc.id, idx)
}

func (jc *Ctx) Cancel(ctx context.Context, idx Idx) error {
	return jc.sys.Cancel(ctx, jc.id, idx)
}

func (jc *Ctx) Inspect(ctx context.Context, idx Idx) (*Job, error) {
	return jc.sys.Inspect(ctx, jc.id, idx)
}

func (jc *Ctx) Errorf(msg string, args ...any) {
	fmt.Fprintf(os.Stderr, jc.id.String()+": "+msg+"\n", args...)
}

func (jc *Ctx) Infof(msg string, args ...any) {
	fmt.Fprintf(os.Stderr, jc.id.String()+": "+msg+"\n", args...)
}

func (jc *Ctx) Debugf(msg string, args ...any) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
}

// Do spawns a child job to compute the Task, then awaits it and returns the result
func Do(ctx context.Context, jc *Ctx, src cadata.Getter, task Task) (*Result, error) {
	id, err := jc.Spawn(ctx, src, task)
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
	return job.Result, nil
}
