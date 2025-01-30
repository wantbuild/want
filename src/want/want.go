package want

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/blobcache/glfs"
	"github.com/jmoiron/sqlx"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/internal/op/dagops"
	"wantbuild.io/want/src/internal/op/goops"
	"wantbuild.io/want/src/internal/op/qemuops"
	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/internal/wantc"
	"wantbuild.io/want/src/internal/wantdag"
	"wantbuild.io/want/src/internal/wantdb"
	"wantbuild.io/want/src/internal/wantsetup"
	"wantbuild.io/want/src/wantjob"
	"wantbuild.io/want/src/wantrepo"
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

func (s *System) qemuDir() string {
	return filepath.Join(s.stateDir, "qemu")
}

func (s *System) logDir() string {
	return filepath.Join(s.stateDir, "log")
}

func (s *System) goDir() string {
	return filepath.Join(s.stateDir, "go")
}

// Init initializes the system
func (s *System) Init(ctx context.Context) error {
	return s.init(ctx, true)
}

func (s *System) init(ctx context.Context, install bool) error {
	if err := s.db.PingContext(ctx); err != nil {
		return err
	}
	if err := wantdb.Setup(ctx, s.db); err != nil {
		return err
	}
	numWorkers := runtime.GOMAXPROCS(0)

	if install {
		earlyJobs := newJobSystem(s.db, s.logDir(), wantsetup.NewExecutor(), numWorkers)
		defer earlyJobs.Shutdown()
		for p, snippet := range map[string]string{
			s.qemuDir(): qemuops.InstallSnippet(),
			s.goDir():   goops.InstallSnippet(),
		} {
			if _, err := os.Stat(p); err == nil {
				continue // TODO: better way to verify the integrity of the install.
			}
			if err := wantsetup.Install(ctx, earlyJobs, p, snippet); err != nil {
				return err
			}
		}
	}

	exec := newExecutor(ExecutorConfig{
		QEMU: QEMUConfig{
			InstallDir: s.qemuDir(),
			MemLimit:   4e9,
		},
		GoDir: s.goDir(),
	})
	s.jobs = newJobSystem(s.db, s.logDir(), exec, numWorkers)
	return nil
}

func (s *System) LogFS() fs.FS {
	return os.DirFS(s.logDir())
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

func (sys *System) ListJobInfos(ctx context.Context) ([]*wantjob.JobInfo, error) {
	return sys.jobs.ListInfos(ctx)
}

func (sys *System) JobSystem() wantjob.System {
	return sys.jobs
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
	return doGLFS(ctx, sys.jobs, s, joinOpName("dag", dagops.OpExecLast), *dagRef)
}

// IsModule returns true if the tree at x is a valid want module.
// All non-tree refs return (false, nil)
func IsModule(ctx context.Context, src cadata.Getter, x glfs.Ref) (bool, error) {
	return wantc.IsModule(ctx, src, x)
}

func doGLFS(ctx context.Context, jobs wantjob.System, src cadata.Getter, op wantjob.OpName, input glfs.Ref) (*glfs.Ref, cadata.Getter, error) {
	res, s, err := wantjob.Do(ctx, jobs, src, wantjob.Task{
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
