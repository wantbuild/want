package wantjob

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
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
	JobState_UNKNOWN JobState = iota
	QUEUED
	RUNNING
	DONE
)

func (js JobState) String() string {
	switch js {
	case JobState_UNKNOWN:
		return "UNKNOWN"
	case QUEUED:
		return "QUEUED"
	case RUNNING:
		return "RUNNING"
	case DONE:
		return "DONE"
	default:
		return "UNKNOWN(" + strconv.Itoa(int(js)) + ")"
	}
}

type ErrCode uint32

func (ec ErrCode) String() string {
	switch ec {
	case OK:
		return "OK"
	case TIMEOUT:
		return "TIMEOUT"
	case CANCELLED:
		return "CANCELLED"
	case EXEC_ERROR:
		return "EXECUTION_ERROR"
	case INTERNAL_ERROR:
		return "INTERNAL_ERROR"
	default:
		return strconv.Itoa(int(ec))
	}
}

const (
	// OK means the job completed successfully
	OK ErrCode = 0
	// TIMEOUT means the system lost contact with the job or it was taking too long
	TIMEOUT = 1
	// CANCELLED means the job was cancelled before it could complete
	CANCELLED = 2

	// EXEC_ERROR is an execution error, directly related to performing the task.
	EXEC_ERROR = 4
	// INTERNAL_ERROR is a system failure, unrelated to the task.
	INTERNAL_ERROR = 5
)

// Result is produced by finished jobs.
// Jobs will also have results when cancelled or timed out, with the situation reflected in the ErrCode
type Result struct {
	ErrCode ErrCode `json:"errcode"`
	Root    []byte  `json:"root"`
	Schema  Schema  `json:"schema"`
}

func Success(schema Schema, data []byte) *Result {
	return &Result{Schema: schema, Root: data}
}

func Result_ErrExec(err error) *Result {
	return &Result{ErrCode: EXEC_ERROR, Root: []byte(err.Error())}
}

func Result_ErrInternal(err error) *Result {
	return &Result{ErrCode: INTERNAL_ERROR, Root: []byte(err.Error())}
}

func (r *Result) Err() error {
	if r.ErrCode == 0 {
		return nil
	}
	return fmt.Errorf("job failed errcode=%v data=%s", r.ErrCode, r.Root)
}

type JobInfo struct {
	ID JobID
	Job
}

type Job struct {
	Task  Task
	State JobState

	Result *Result

	CreatedAt tai64.TAI64N
	StartAt   *tai64.TAI64N
	EndAt     *tai64.TAI64N
}

func (j Job) Elapsed() time.Duration {
	if j.EndAt == nil {
		return time.Since(j.StartAt.GoTime())
	}
	return j.EndAt.GoTime().Sub(j.StartAt.GoTime())
}

// System manages spawning, running, and awaiting jobs.
type System interface {
	// Spawn creates a new job, which will perform task pulling data from src as needed.
	// Job returns an index, which can be used to refer to the Job in other methods.
	Spawn(ctx context.Context, src cadata.Getter, task Task) (Idx, error)
	// Inspect returns information about a Job
	Inspect(ctx context.Context, idx Idx) (*Job, error)
	// Await blocks until the job has completed
	Await(ctx context.Context, idx Idx) error
	// Cancel attempts to cancel the job.  If the Job is already in DONE then it cannot be cancelled.
	Cancel(ctx context.Context, idx Idx) error
	// ViewResult returns the Job result, or immediately errors if the Job is not in the DONE state.
	ViewResult(ctx context.Context, idx Idx) (*Result, cadata.Getter, error)
	// Delete makes the Job inaccessbile and frees any resources it is consuming.
	// Delete will also cancel the Job if it is running, although this transitory state will
	// not be visible to the caller of Delete.
	Delete(ctx context.Context, idx Idx) error
}

var _ System = Ctx{}

// Ctx is a Job Context.  It is the API available from within a running job
type Ctx struct {
	Context context.Context
	System
	Dst    cadata.Store
	Writer func(string) io.Writer
}

func (jc *Ctx) Errorf(msg string, args ...any) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
}

func (jc *Ctx) Infof(msg string, args ...any) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
}

func (jc *Ctx) Debugf(msg string, args ...any) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
}

func (jc *Ctx) InfoSpan(msg string) func() {
	jc.Infof("%s: begin", msg)
	startTime := time.Now()
	return func() { jc.Infof("%s: end %v", msg, time.Since(startTime)) }
}

// Do spawns a child job to compute the Task, then awaits it and returns the result
func Do(ctx context.Context, sys System, src cadata.Getter, task Task) (*Result, cadata.Getter, error) {
	idx, err := sys.Spawn(ctx, src, task)
	if err != nil {
		return nil, nil, err
	}
	if err := sys.Await(ctx, idx); err != nil {
		return nil, nil, err
	}
	return sys.ViewResult(ctx, idx)
}
