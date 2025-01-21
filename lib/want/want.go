package want

import (
	"context"
	"os"
	"path/filepath"
	"runtime"

	"github.com/blobcache/glfs"
	"github.com/jmoiron/sqlx"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/glfstasks"
	"wantbuild.io/want/internal/op/dagops"
	"wantbuild.io/want/internal/op/qemuops"
	"wantbuild.io/want/internal/stores"
	"wantbuild.io/want/internal/wantc"
	"wantbuild.io/want/internal/wantdag"
	"wantbuild.io/want/internal/wantdb"
	"wantbuild.io/want/internal/wantsetup"
	"wantbuild.io/want/lib/wantjob"
	"wantbuild.io/want/lib/wantrepo"
)

// System is an instance of the Want Build System
type System struct {
	stateDir   string
	numWorkers int

	db   *sqlx.DB
	jobs *jobSystem
}

func New(stateDir string, numWorkers int) *System {
	db, err := wantdb.Open(filepath.Join(stateDir, "want.db"))
	if err != nil {
		panic(err)
	}
	return &System{
		stateDir:   stateDir,
		numWorkers: numWorkers,

		db: db,
	}
}

func (s *System) qemuPath() string {
	return filepath.Join(s.stateDir, "qemu")
}

// Init initializes the system
func (s *System) Init(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return err
	}
	if err := wantdb.Setup(ctx, s.db); err != nil {
		return err
	}
	numWorkers := runtime.GOMAXPROCS(0)
	earlyJobs := newJobSystem(s.db, wantsetup.NewExecutor(), numWorkers)
	defer earlyJobs.Shutdown()
	for p, snippet := range map[string]string{
		s.qemuPath(): qemuops.InstallSnippet(),
	} {
		if _, err := os.Stat(p); err == nil {
			continue // TODO: better way to verify the integrity of the install.
		}
		if err := wantsetup.Install(ctx, earlyJobs, p, snippet); err != nil {
			return err
		}
	}

	s.jobs = newJobSystem(s.db, newExecutor(s.qemuPath(), 4e9), numWorkers)
	return nil
}

func (s *System) Close() error {
	if s.jobs != nil {
		s.jobs.Shutdown()
		s.jobs = nil
	}
	if err := s.db.Close(); err != nil {
		return err
	}
	s.db = nil
	return nil
}

func (sys *System) Eval(ctx context.Context, db *sqlx.DB, repo *wantrepo.Repo, calledFrom string, expr []byte) (*glfs.Ref, cadata.Getter, error) {
	s := stores.NewMem()

	c := wantc.NewCompiler()
	dag, err := c.CompileSnippet(ctx, s, s, expr)
	if err != nil {
		return nil, nil, err
	}
	dagRef, err := wantdag.PostDAG(ctx, s, *dag)
	if err != nil {
		return nil, nil, err
	}
	return sys.doGLFS(ctx, s, joinOpName("dag", dagops.OpExecLast), *dagRef)
}

func (sys *System) doGLFS(ctx context.Context, src cadata.Getter, op wantjob.OpName, input glfs.Ref) (*glfs.Ref, cadata.Getter, error) {
	res, s, err := wantjob.Do(ctx, sys.jobs, src, wantjob.Task{
		Op:    op,
		Input: glfstasks.MarshalGLFSRef(input),
	})
	if err != nil {
		return nil, nil, err
	}
	if err := res.Err(); err != nil {
		return nil, nil, err
	}
	ref, err := glfstasks.ParseGLFSRef(res.Data)
	if err != nil {
		return nil, nil, err
	}
	return ref, s, nil
}

func joinOpName(xs ...wantjob.OpName) (ret wantjob.OpName) {
	for i, x := range xs {
		if i > 0 {
			ret += "."
		}
		ret += x
	}
	return ret
}
