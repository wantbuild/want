package want

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/jmoiron/sqlx"
	"go.brendoncarroll.net/state/cadata"
	"go.brendoncarroll.net/tai64"

	"wantbuild.io/want/internal/dbutil"
	"wantbuild.io/want/internal/op/dagops"
	"wantbuild.io/want/internal/op/glfsops"
	"wantbuild.io/want/internal/op/importops"
	"wantbuild.io/want/internal/op/wantops"
	"wantbuild.io/want/internal/singleflight"
	"wantbuild.io/want/internal/wantdb"
	"wantbuild.io/want/lib/wantjob"
)

var _ wantjob.System = &job{}

type job struct {
	sys *jobSystem
	id  wantjob.JobID

	task wantjob.Task
	src  cadata.Getter

	ctx    context.Context
	cf     context.CancelFunc
	dst    cadata.Store
	done   chan struct{}
	result *wantjob.Result

	childMu  sync.RWMutex
	children []*job

	createdAt, startAt, endAt tai64.TAI64N
}

func newJob(sys *jobSystem, parent *job, dst cadata.Store, src cadata.Getter, task wantjob.Task) *job {
	parentCtx := sys.bgCtx
	if parent != nil {
		parentCtx = parent.ctx
	}
	ctx, cf := context.WithCancel(parentCtx)
	return &job{
		sys:  sys,
		task: task,
		dst:  dst,
		src:  src,

		ctx:       ctx,
		cf:        cf,
		done:      make(chan struct{}),
		createdAt: tai64.Now(),
	}
}

func (j *job) Spawn(ctx context.Context, src cadata.Getter, task wantjob.Task) (wantjob.Idx, error) {
	idx, child, err := j.sys.spawn(ctx, j, src, task)
	if err != nil {
		return 0, err
	}
	idx2 := j.addChild(child)
	if idx != idx2 {
		panic(idx2)
	}
	return idx, nil
}

func (j *job) Cancel(ctx context.Context, idx wantjob.Idx) error {
	child, err := j.getChild(idx)
	if err != nil {
		return err
	}
	return child.cancel()
}

func (j *job) Await(ctx context.Context, idx wantjob.Idx) error {
	child, err := j.getChild(idx)
	if err != nil {
		return err
	}
	return child.await(ctx)
}

func (j *job) Inspect(ctx context.Context, idx wantjob.Idx) (*wantjob.Job, error) {
	child, err := j.getChild(idx)
	if err != nil {
		return nil, err
	}
	return child.inspect(), nil
}

func (j *job) ViewResult(ctx context.Context, idx wantjob.Idx) (*wantjob.Result, cadata.Getter, error) {
	child, err := j.getChild(idx)
	if err != nil {
		return nil, nil, err
	}
	return child.viewResult()
}

func (j *job) getChild(idx wantjob.Idx) (*job, error) {
	j.childMu.RLock()
	defer j.childMu.RUnlock()
	if len(j.children) <= int(idx) {
		return nil, fmt.Errorf("job not found: %v", idx)
	}
	return j.children[idx], nil
}

func (j *job) addChild(child *job) wantjob.Idx {
	j.childMu.Lock()
	defer j.childMu.Unlock()
	idx := wantjob.Idx(len(j.children))
	j.children = append(j.children, child)

	child.id = append(slices.Clone(j.id), idx)
	return idx
}

func (j *job) await(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-j.done:
		return nil
	}
}

func (j *job) inspect() *wantjob.Job {
	var (
		endAt *tai64.TAI64N
		res   *wantjob.Result
	)
	if j.isDone() {
		endAt = &j.endAt
		res = j.result
	}
	return &wantjob.Job{
		Task:      j.task,
		CreatedAt: j.createdAt,
		EndAt:     endAt,
		Result:    res,
	}
}

func (j *job) cancel() error {
	panic("cancel not implemented")
}

func (j *job) viewResult() (*wantjob.Result, cadata.Getter, error) {
	if !j.isDone() {
		return nil, nil, fmt.Errorf("job is not done")
	}
	return j.result, j.dst, nil
}

func (js *job) isDone() bool {
	select {
	case <-js.done:
		return true
	default:
		return false
	}
}

type jobSystem struct {
	db   *sqlx.DB
	exec wantjob.Executor

	bgCtx context.Context
	cf    context.CancelFunc

	mu       sync.RWMutex
	rootJobs map[wantjob.Idx]*job

	queue chan *job
	wg    sync.WaitGroup
}

func newJobSystem(db *sqlx.DB, exec wantjob.Executor, numWorkers int) *jobSystem {
	bgCtx, cf := context.WithCancel(context.Background())
	s := &jobSystem{
		db:   db,
		exec: exec,

		bgCtx: bgCtx,
		cf:    cf,

		rootJobs: make(map[wantjob.Idx]*job),

		queue: make(chan *job),
	}

	s.wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func() {
			defer s.wg.Done()
			for {
				select {
				case <-s.bgCtx.Done():
					return
				case x, ok := <-s.queue:
					if !ok {
						return
					}
					if err := s.process(x); err != nil {
						panic(err) // TODO: need other way to signal internal failure
					}
				}
			}
		}()
	}
	return s
}

func (sys *jobSystem) Spawn(ctx context.Context, src cadata.Getter, task wantjob.Task) (wantjob.Idx, error) {
	idx, j, err := sys.spawn(ctx, nil, src, task)
	if err != nil {
		return 0, err
	}
	sys.mu.Lock()
	defer sys.mu.Unlock()
	sys.rootJobs[idx] = j
	return idx, nil
}

func (sys *jobSystem) Inspect(ctx context.Context, idx wantjob.Idx) (*wantjob.Job, error) {
	sys.mu.RLock()
	j, exists := sys.rootJobs[idx]
	sys.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("job not found %v", idx)
	}
	return j.inspect(), nil
}

func (sys *jobSystem) Await(ctx context.Context, idx wantjob.Idx) error {
	sys.mu.RLock()
	j, exists := sys.rootJobs[idx]
	sys.mu.RUnlock()
	if !exists {
		return fmt.Errorf("job not found %v", idx)
	}
	return j.await(ctx)
}

func (sys *jobSystem) Cancel(ctx context.Context, idx wantjob.Idx) error {
	sys.mu.RLock()
	defer sys.mu.RUnlock()
	j, exists := sys.rootJobs[idx]
	if !exists {
		return fmt.Errorf("job not found %v", idx)
	}
	if j.isDone() {
		return fmt.Errorf("job is already finished cannot cancel")
	}
	return j.cancel()
}

func (sys *jobSystem) ViewResult(ctx context.Context, idx wantjob.Idx) (*wantjob.Result, cadata.Getter, error) {
	sys.mu.RLock()
	defer sys.mu.RUnlock()
	j, exists := sys.rootJobs[idx]
	if !exists {
		return nil, nil, fmt.Errorf("job not found %v", idx)
	}
	return j.viewResult()
}

func (sys *jobSystem) spawn(ctx context.Context, parent *job, src cadata.Getter, task wantjob.Task) (wantjob.Idx, *job, error) {
	var (
		idx   wantjob.Idx
		dbJob *wantjob.Job
		dstID wantdb.StoreID
		jobid wantjob.JobID
	)
	if err := dbutil.DoTx(ctx, sys.db, func(tx *sqlx.Tx) error {
		var err error
		if parent == nil {
			if idx, err = wantdb.CreateRootJob(tx, task); err != nil {
				return err
			}
			jobid = wantjob.JobID{idx}
		} else {
			if idx, err = wantdb.CreateChildJob(tx, parent.id, task); err != nil {
				return err
			}
			jobid = append(slices.Clone(parent.id), idx)
		}
		if dbJob, err = wantdb.InspectJob(tx, jobid); err != nil {
			return err
		}
		if dstID, err = wantdb.GetJobStoreID(tx, jobid); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return 0, nil, err
	}

	dst := wantdb.NewDBStore(sys.db, dstID)
	j := newJob(sys, parent, dst, src, task)
	j.id = jobid
	sys.maybeEnqueue(j, dbJob)
	return idx, j, nil
}

// maybeEnqueue enqueues the job if it was advanced to DONE using the cache.
func (s *jobSystem) maybeEnqueue(jstate *job, dbJob *wantjob.Job) {
	switch dbJob.State {
	case wantjob.QUEUED:
		s.queue <- jstate
	case wantjob.DONE:
		jstate.result = dbJob.Result
		jstate.endAt = tai64.Now()
		close(jstate.done)
	}
}

func (s *jobSystem) process(x *job) error {
	res := func() wantjob.Result {
		jc := wantjob.Ctx{Context: x.ctx, Dst: x.dst, System: x}
		out, err := s.exec.Execute(jc, x.src, x.task)
		if err != nil {
			return *wantjob.Result_ErrExec(err)
		}
		return *wantjob.Success(out)
	}()
	if err := s.finishJob(x.ctx, x.id, res); err != nil {
		return err
	}
	x.result = &res
	x.endAt = tai64.Now()
	close(x.done)
	return nil
}

func (s *jobSystem) finishJob(ctx context.Context, jobid wantjob.JobID, res wantjob.Result) error {
	return dbutil.DoTx(ctx, s.db, func(tx *sqlx.Tx) error {
		return wantdb.FinishJob(tx, jobid, res)
	})
}

func (s *jobSystem) Shutdown() {
	s.cf()
	close(s.queue)
	for x := range s.queue {
		x.cf()
	}
	s.wg.Wait()
}

type executor struct {
	execs map[wantjob.OpName]wantjob.Executor
	sf    singleflight.Group[wantjob.TaskID, []byte]
}

func newProtoExecutor() *executor {
	glfsExec := glfsops.Executor{}
	wantExec := wantops.Executor{}
	dagExec := dagops.Executor{}
	impExec := importops.NewExecutor()

	return &executor{
		execs: map[wantjob.OpName]wantjob.Executor{
			"glfs":   glfsExec,
			"want":   wantExec,
			"dag":    dagExec,
			"import": impExec,
		},
	}
}

func (e *executor) Execute(jc wantjob.Ctx, src cadata.Getter, task wantjob.Task) ([]byte, error) {
	parts := strings.SplitN(string(task.Op), ".", 2)
	e2, exists := e.execs[wantjob.OpName(parts[0])]
	if !exists {
		return nil, wantjob.ErrOpNotFound{Op: task.Op}
	}
	out, err, _ := e.sf.Do(task.ID(), func() ([]byte, error) {
		return e2.Execute(jc, src, wantjob.Task{
			Op:    wantjob.OpName(parts[1]),
			Input: task.Input,
		})
	})
	return out, err
}
