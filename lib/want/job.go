package want

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/blobcache/glfs"
	"github.com/jmoiron/sqlx"
	"go.brendoncarroll.net/state/cadata"
	"go.brendoncarroll.net/tai64"

	"wantbuild.io/want/internal/dbutil"
	"wantbuild.io/want/internal/wantdb"
	"wantbuild.io/want/internal/wantjob"
)

var _ wantjob.System = &JobSys{}

type jobState struct {
	id      wantjob.JobID
	task    wantjob.Task
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

func newJobState(bgCtx context.Context, task wantjob.Task) *jobState {
	ctx, cf := context.WithCancel(bgCtx)
	return &jobState{
		task:    task,
		startAt: tai64.Now(),

		ctx:  ctx,
		cf:   cf,
		done: make(chan struct{}),
	}
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
	s    cadata.Store

	bgCtx context.Context
	cf    context.CancelFunc

	mu    sync.RWMutex
	roots map[wantjob.Idx]*jobState

	queue chan *jobState
}

func newJobSys(bgCtx context.Context, db *sqlx.DB, exec wantjob.Executor, s cadata.Store, numWorkers int) *JobSys {
	bgCtx, cf := context.WithCancel(bgCtx)
	sys := &JobSys{
		db:   db,
		exec: exec,
		s:    s,

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

func (s *JobSys) cacheRead(ctx context.Context, tid wantjob.TaskID) []byte {
	outData, err := dbutil.DoTx1(ctx, s.db, func(tx *sqlx.Tx) ([]byte, error) {
		return wantdb.CacheRead(tx, tid)
	})
	if err != nil {
		outData = nil
	}
	return outData
}

func (s *JobSys) finishJob(ctx context.Context, jobid wantjob.JobID, res wantjob.Result) error {
	var resData []byte
	if !res.Data.CID.IsZero() {
		var err error
		resData, err = json.Marshal(res.Data)
		if err != nil {
			return err
		}
	}
	return dbutil.DoTx(ctx, s.db, func(tx *sqlx.Tx) error {
		return wantdb.FinishJob(tx, jobid, res.ErrCode, resData)
	})
}

func (s *JobSys) process(x *jobState) {
	defer close(x.done)
	x.dequeued.Store(true)

	// check cache
	res := func() wantjob.Result {
		tid := x.task.ID()
		if outData := s.cacheRead(x.ctx, tid); len(outData) > 0 {
			var outRoot glfs.Ref
			if err := json.Unmarshal(outData, &outRoot); err != nil {
				return wantjob.Result{
					ErrCode: wantjob.ErrCode_EXEC,
				}
			}
			return wantjob.Result{
				Data: outRoot,
			}
		}
		// compute
		jc := wantjob.NewCtx(s, x.id)
		out, err := s.exec.Compute(x.ctx, &jc, s.s, x.task)
		if err != nil {
			return wantjob.Result{
				// TODO: encode error as data
				ErrCode: wantjob.ErrCode_EXEC,
			}
		} else {
			return wantjob.Result{
				Data: *out,
			}
		}
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

func (s *JobSys) Init(ctx context.Context, task wantjob.Task) (wantjob.Idx, error) {
	rootIdx, err := dbutil.DoTx1(ctx, s.db, func(tx *sqlx.Tx) (wantjob.Idx, error) {
		return wantdb.CreateRootJob(tx, task)
	})
	if err != nil {
		return 0, nil
	}
	js := newJobState(s.bgCtx, task)
	js.id = wantjob.JobID{rootIdx}

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

	child := newJobState(context.Background(), task)
	idx := ps.addChild(child)
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
