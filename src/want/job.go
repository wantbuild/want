package want

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"sync"

	"github.com/jmoiron/sqlx"
	"go.brendoncarroll.net/exp/singleflight"
	"go.brendoncarroll.net/state/cadata"
	"go.brendoncarroll.net/stdctx/logctx"
	"go.brendoncarroll.net/tai64"
	"go.uber.org/zap"

	"wantbuild.io/want/src/internal/dbutil"
	"wantbuild.io/want/src/internal/wantdb"
	"wantbuild.io/want/src/wantjob"
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

	lwMu       sync.Mutex
	logWriters map[string]*os.File

	createdAt, startAt, endAt tai64.TAI64N
	stackTrace                []byte
}

func newJob(sys *jobSystem, parent *job, idx wantjob.Idx, dst cadata.Store, src cadata.Getter, task wantjob.Task) *job {
	var jobid wantjob.JobID
	parentCtx := sys.bgCtx
	if parent != nil {
		parentCtx = parent.ctx
		jobid = slices.Clone(parent.id)
	}
	jobid = append(jobid, idx)
	ctx, cf := context.WithCancel(parentCtx)
	return &job{
		id:   jobid,
		sys:  sys,
		task: task,
		dst:  dst,
		src:  src,

		ctx:  ctx,
		cf:   cf,
		done: make(chan struct{}),

		createdAt:  tai64.Now(),
		stackTrace: debug.Stack(),
	}
}

func (j *job) Spawn(ctx context.Context, src cadata.Getter, task wantjob.Task) (wantjob.Idx, error) {
	j.childMu.Lock()
	defer j.childMu.Unlock()
	idx, child, err := j.sys.spawn(ctx, j, src, task)
	if err != nil {
		return 0, err
	}
	j.addChild(idx, child)
	return idx, nil
}

func (j *job) Cancel(ctx context.Context, idx wantjob.Idx) error {
	child, err := j.getChild(idx)
	if err != nil {
		return err
	}
	return child.cancel()
}

func (j *job) Delete(ctx context.Context, idx wantjob.Idx) error {
	return errors.New("delete not implemented")
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

func (j *job) Writer(topic string) io.Writer {
	j.lwMu.Lock()
	defer j.lwMu.Unlock()
	if j.logWriters == nil {
		j.logWriters = make(map[string]*os.File)
	}
	lw, exists := j.logWriters[topic]
	if !exists {
		f, err := j.sys.openLogFile(j.id, topic)
		if err != nil {
			panic(err)
		}
		lw = f
		j.logWriters[topic] = lw
	}
	return io.MultiWriter(lw, os.Stderr)
}

func (j *job) getChild(idx wantjob.Idx) (*job, error) {
	j.childMu.RLock()
	defer j.childMu.RUnlock()
	if len(j.children) <= int(idx) {
		return nil, fmt.Errorf("job not found: %v", idx)
	}
	return j.children[idx], nil
}

func (j *job) addChild(idx wantjob.Idx, child *job) {
	idx2 := wantjob.Idx(len(j.children))
	if idx2 != idx {
		panic(idx2)
	}
	j.children = append(j.children, child)
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

func (j *job) finish(ctx context.Context, res wantjob.Result) {
	for k, w := range j.logWriters {
		if err := w.Close(); err != nil {
			logctx.Error(ctx, "closing log writer", zap.Any("job", j.id), zap.String("topic", k), zap.Error(err))
		}
		delete(j.logWriters, k)
	}
	j.result = &res
	j.endAt = tai64.Now()
	close(j.done)
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
	db     *sqlx.DB
	logDir string
	exec   wantjob.Executor
	og     onceGroup[wantjob.TaskID, wantjob.Result]

	bgCtx context.Context
	cf    context.CancelFunc

	mu       sync.RWMutex
	rootJobs map[wantjob.Idx]*job

	queue chan *job
	wg    sync.WaitGroup
}

func newJobSystem(db *sqlx.DB, logDir string, exec wantjob.Executor, numWorkers int) *jobSystem {
	bgCtx, cf := context.WithCancel(context.Background())
	s := &jobSystem{
		db:     db,
		logDir: logDir,
		exec:   exec,

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

func (sys *jobSystem) Delete(ctx context.Context, idx wantjob.Idx) error {
	sys.mu.Lock()
	defer sys.mu.Unlock()
	j, exists := sys.rootJobs[idx]
	if exists {
		if err := j.cancel(); err != nil {
			return err
		}
		if err := dbutil.DoTx(ctx, sys.db, func(tx *sqlx.Tx) error {
			return wantdb.DropJob(tx, wantjob.JobID{idx})
		}); err != nil {
			return err
		}
		delete(sys.rootJobs, idx)
	}
	return nil
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

func (sys *jobSystem) ListInfos(ctx context.Context) ([]*wantjob.JobInfo, error) {
	return dbutil.ROTx1(ctx, sys.db, func(tx *sqlx.Tx) ([]*wantjob.JobInfo, error) {
		return wantdb.ListJobInfos(tx, nil)
	})
}

func (sys *jobSystem) spawn(ctx context.Context, parent *job, src cadata.Getter, task wantjob.Task) (wantjob.Idx, *job, error) {
	var (
		idx   wantjob.Idx
		dbJob *wantjob.Job
		dstID wantdb.StoreID
	)
	if err := dbutil.DoTx(ctx, sys.db, func(tx *sqlx.Tx) error {
		var err error
		var jobid wantjob.JobID
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
	j := newJob(sys, parent, idx, dst, src, task)
	sys.maybeEnqueue(j, dbJob)
	return idx, j, nil
}

// maybeEnqueue enqueues the job if it is still running
// (it was not advanced to directly to DONE using the cache.)
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

func (s *jobSystem) process(x *job) (retErr error) {
	defer func() {
		if retErr != nil {
			x.finish(s.bgCtx, *wantjob.Result_ErrInternal(retErr))
		}
	}()
	taskID := x.task.ID()
	var original bool
	res, err := s.og.Do(taskID, func() (wantjob.Result, error) {
		original = true
		jc := wantjob.Ctx{
			Context: x.ctx,
			Dst:     x.dst,
			System:  x,
			Writer:  x.Writer,
		}
		res := s.exec.Execute(jc, x.src, x.task)

		// we have to complete the job in the database here because down below
		// we do a Pull, and there needs to be a completed job to pull from.
		// without this, there is a race that can cause errors.
		if err := s.finishJob(s.bgCtx, x.id, res); err != nil {
			return *wantjob.Result_ErrInternal(err), nil
		}
		return res, nil
	})
	if err != nil {
		return err
	}
	// if it was not originally computed, and the output is successful GLFS, then
	// we need to Pull into the job's store.
	if !original && res.ErrCode == 0 {
		if err := x.dst.(*wantdb.DBStore).Pull(s.bgCtx, res.Root); err != nil {
			return err
		}
		if err := s.finishJob(s.bgCtx, x.id, res); err != nil {
			return err
		}
	}

	x.finish(s.bgCtx, res)
	return nil
}

// finishJob finishes the job in the database
func (s *jobSystem) finishJob(ctx context.Context, jobid wantjob.JobID, res wantjob.Result) error {
	return dbutil.DoTx(ctx, s.db, func(tx *sqlx.Tx) error {
		return wantdb.FinishJob(tx, jobid, res)
	})
}

func (s *jobSystem) openLogFile(id wantjob.JobID, topic string) (*os.File, error) {
	parts := []string{s.logDir}
	for _, idx := range id {
		parts = append(parts, idx.String())
	}
	parts = append(parts, topic)
	p := filepath.Join(parts...)

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
}

func (s *jobSystem) Shutdown() {
	s.cf()
	close(s.queue)
	for x := range s.queue {
		x.cf()
	}
	s.wg.Wait()
}

type onceGroup[K comparable, V any] struct {
	sf    singleflight.Group[K, V]
	mu    sync.RWMutex
	cache map[K]V
}

func (og *onceGroup[K, V]) Do(k K, fn func() (V, error)) (V, error) {
	og.mu.RLock()
	val, exists := og.cache[k]
	og.mu.RUnlock()
	if exists {
		return val, nil
	}

	val, err, _ := og.sf.Do(k, func() (V, error) {
		og.mu.RLock()
		val, exists := og.cache[k]
		og.mu.RUnlock()
		if exists {
			return val, nil
		}
		val, err := fn()
		if err == nil {
			og.mu.Lock()
			if og.cache == nil {
				og.cache = make(map[K]V)
			}
			og.cache[k] = val
			og.mu.Unlock()
		}
		return val, err
	})
	return val, err
}
