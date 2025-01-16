package want

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/blobcache/glfs"
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
	"wantbuild.io/want/internal/wantjob"
)

var _ wantjob.System = &JobSys{}

type jobState struct {
	id      wantjob.JobID
	task    wantjob.Task
	src     cadata.Getter
	dst     cadata.Store
	startAt tai64.TAI64N

	ctx      context.Context
	cf       context.CancelFunc
	dequeued atomic.Bool
	done     chan struct{}
	endAt    tai64.TAI64N
	result   *wantjob.Result

	childMu  sync.RWMutex
	children []*jobState
}

func newJobState(bgCtx context.Context, src cadata.Getter, task wantjob.Task) *jobState {
	ctx, cf := context.WithCancel(bgCtx)
	return &jobState{
		task:    task,
		src:     src,
		startAt: tai64.Now(),

		ctx:  ctx,
		cf:   cf,
		done: make(chan struct{}),
	}
}

func (js *jobState) createChild(task wantjob.Task) (wantjob.Idx, *jobState) {
	child := newJobState(js.ctx, js.dst, task)
	return js.addChild(child), child
}

func (js *jobState) getChild(idx wantjob.Idx) *jobState {
	js.childMu.RLock()
	defer js.childMu.RUnlock()
	if len(js.children) <= int(idx) {
		return nil
	}
	return js.children[idx]
}

func (js *jobState) addChild(child *jobState) wantjob.Idx {
	js.childMu.Lock()
	defer js.childMu.Unlock()
	idx := wantjob.Idx(len(js.children))
	js.children = append(js.children, child)

	child.id = append(slices.Clone(js.id), idx)
	child.dst = js.dst
	return idx
}

func (js *jobState) inspect() *wantjob.Job {
	var (
		endAt *tai64.TAI64N
		res   *wantjob.Result
	)
	if js.isDone() {
		endAt = &js.endAt
		res = js.result
	}
	return &wantjob.Job{
		Task:    js.task,
		StartAt: js.startAt,
		EndAt:   endAt,
		Result:  res,
	}
}

func (js *jobState) isDone() bool {
	select {
	case <-js.done:
		return true
	default:
		return false
	}
}

type JobSys struct {
	db   *sqlx.DB
	exec wantjob.Executor

	bgCtx context.Context
	cf    context.CancelFunc

	mu    sync.RWMutex
	roots map[wantjob.Idx]*jobState

	queue chan *jobState
}

func newJobSys(bgCtx context.Context, db *sqlx.DB, exec wantjob.Executor, numWorkers int) *JobSys {
	bgCtx, cf := context.WithCancel(bgCtx)
	sys := &JobSys{
		db:   db,
		exec: exec,

		bgCtx: bgCtx,
		cf:    cf,

		roots: make(map[wantjob.Idx]*jobState),
		queue: make(chan *jobState, 1024),
	}
	for i := 0; i < numWorkers; i++ {
		go func() {
			for {
				select {
				case <-bgCtx.Done():
					return
				case x := <-sys.queue:
					sys.process(x)
				}
			}
		}()
	}
	return sys
}

func (s *JobSys) finishJob(ctx context.Context, jobid wantjob.JobID, res wantjob.Result) error {
	return dbutil.DoTx(ctx, s.db, func(tx *sqlx.Tx) error {
		return wantdb.FinishJob(tx, jobid, res)
	})
}

func (s *JobSys) process(x *jobState) {
	defer close(x.done)
	x.dequeued.Store(true)

	res := func() wantjob.Result {
		// TODO: check cache
		// compute
		jc := wantjob.NewCtx(s, x.id)
		out, err := s.exec.Compute(x.ctx, &jc, x.src, x.task)
		if err != nil {
			return *wantjob.Result_ErrExec(err)
		}
		if err := glfs.Sync(x.ctx, x.dst, s.exec.GetStore(), *out); err != nil {
			return *wantjob.Result_ErrExec(err)
		}
		data, err := json.Marshal(out)
		if err != nil {
			panic(err)
		}
		return *wantjob.Succeed(data)
	}()
	if err := s.finishJob(x.ctx, x.id, res); err != nil {
		panic(err) // TODO: need other way to signal internal failure
	}
	x.result = &res
	x.endAt = tai64.Now()
}

func (s *JobSys) Shutdown() {
	s.cf()
	close(s.queue)
	for x := range s.queue {
		x.cf()
	}
}

func (s *JobSys) Init(ctx context.Context, src cadata.Getter, task wantjob.Task) (wantjob.Idx, error) {
	rootIdx, err := dbutil.DoTx1(ctx, s.db, func(tx *sqlx.Tx) (wantjob.Idx, error) {
		return wantdb.CreateRootJob(tx, task)
	})
	if err != nil {
		return 0, err
	}
	sid, err := dbutil.DoTx1(ctx, s.db, func(tx *sqlx.Tx) (wantdb.StoreID, error) {
		return wantdb.GetJobStoreID(tx, wantjob.JobID{rootIdx})
	})
	if err != nil {
		return 0, err
	}
	dst := wantdb.NewDBStore(s.db, sid)
	if err := glfs.Sync(ctx, dst, src, task.Input); err != nil {
		return 0, err
	}

	js := newJobState(s.bgCtx, src, task)
	js.id = wantjob.JobID{rootIdx}
	js.dst = dst

	s.mu.Lock()
	defer s.mu.Unlock()
	s.roots[rootIdx] = js
	s.queue <- js
	return rootIdx, nil
}

func (s *JobSys) Spawn(ctx context.Context, parent wantjob.JobID, task wantjob.Task) (wantjob.Idx, error) {
	ps := s.getJobState(parent)
	if ps == nil {
		return 0, fmt.Errorf("parent %v not found", parent)
	}

	idx, err := dbutil.DoTx1(ctx, s.db, func(tx *sqlx.Tx) (wantjob.Idx, error) {
		return wantdb.CreateChildJob(tx, ps.id, task)
	})
	if err != nil {
		return 0, err
	}
	idx2, child := ps.createChild(task)
	if idx != idx2 {
		panic("jobidx mismatch")
	}
	s.queue <- child
	return idx, nil
}

func (s *JobSys) Cancel(ctx context.Context, parent wantjob.JobID, idx wantjob.Idx) error {
	x := s.getJobState(append(parent, idx))
	x.cf()
	return nil
}

func (s *JobSys) Inspect(ctx context.Context, parent wantjob.JobID, idx wantjob.Idx) (*wantjob.Job, error) {
	x := s.getJobState(append(parent, idx))
	if x != nil {
		return x.inspect(), nil
	}
	return dbutil.DoTx1(ctx, s.db, func(tx *sqlx.Tx) (*wantjob.Job, error) {
		return wantdb.InspectJob(tx, append(parent, idx))
	})
}

func (s *JobSys) Await(ctx context.Context, parent wantjob.JobID, idx wantjob.Idx) error {
	x := s.getJobState(append(parent, idx))
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-x.done:
		return nil
	}
}

func (s *JobSys) getJobState(jobid wantjob.JobID) *jobState {
	if len(jobid) == 0 {
		return nil
	}
	s.mu.RLock()
	x := s.roots[jobid[0]]
	s.mu.RUnlock()
	for _, idx := range jobid[1:] {
		x = x.getChild(idx)
	}
	return x
}

type executor struct {
	s     cadata.GetPoster
	execs map[wantjob.OpName]wantjob.Executor
	sf    singleflight.Group[wantjob.Task, *glfs.Ref]
}

func newExecutor(s cadata.Store) *executor {
	glfsExec := glfsops.NewExecutor(s)
	wantExec := wantops.NewExecutor(s)
	dagExec := dagops.NewExecutor(s)
	impExec := importops.NewExecutor(s)

	return &executor{
		s: s,
		execs: map[wantjob.OpName]wantjob.Executor{
			"glfs":   glfsExec,
			"want":   wantExec,
			"dag":    dagExec,
			"import": impExec,
		},
	}
}

func (e *executor) Compute(ctx context.Context, jc *wantjob.Ctx, src cadata.Getter, task wantjob.Task) (*glfs.Ref, error) {
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
