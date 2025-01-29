package goops

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"
	"go.brendoncarroll.net/stdctx/logctx"
	"golang.org/x/sync/semaphore"

	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/wantjob"
)

const (
	OpModDownload = wantjob.OpName("modDownload")

	OpMakeExec     = wantjob.OpName("makeExec")
	OpMakeTestExec = wantjob.OpName("makeTestExec")
)

const (
	goVersion = "1.23.4"
)

var _ wantjob.Executor = &Executor{}

type Executor struct {
	installDir string
	buildSem   *semaphore.Weighted
}

func NewExecutor(installDir string) *Executor {
	return &Executor{
		installDir: installDir,
		buildSem:   semaphore.NewWeighted(int64(runtime.GOMAXPROCS(0))),
	}
}

func (e *Executor) Execute(jc wantjob.Ctx, src cadata.Getter, task wantjob.Task) ([]byte, error) {
	ctx := jc.Context
	if err := e.buildSem.Acquire(ctx, 1); err != nil {
		return nil, err
	}
	defer e.buildSem.Release(1)
	switch task.Op {
	case OpMakeExec:
		return glfstasks.Exec(task.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			met, err := GetMakeExecTask(ctx, src, x)
			if err != nil {
				return nil, err
			}
			return e.MakeExec(jc, src, *met)
		})
	case OpMakeTestExec:
		return glfstasks.Exec(task.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			mtet, err := GetMakeTestExecTask(ctx, src, x)
			if err != nil {
				return nil, err
			}
			return e.MakeTestExec(jc, src, *mtet)
		})
	case OpModDownload:
		return glfstasks.Exec(task.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			return e.ModDownload(jc, src, x)
		})
	default:
		return nil, wantjob.NewErrUnknownOperator(task.Op)
	}
}

type goConfig struct {
	GOARCH     string
	GOOS       string
	GOMODCACHE string
	GOPROXY    string
}

func (e *Executor) newCommand(ctx context.Context, cfg goConfig, args ...string) *exec.Cmd {
	goRoot := filepath.Join(e.installDir)
	cmd := exec.CommandContext(ctx, filepath.Join(goRoot, "bin", "go"), args...)
	cmd.Env = []string{
		"PATH=/usr/bin/",
		"CGO_ENABLED=0",
		"GOSUMDB=off",

		"GOROOT=" + goRoot,
		"GOPATH=" + filepath.Join(e.installDir, "gopath"),
		//"GOCACHE=" + filepath.Join(e.installDir, "gocache"),

		"GOARCH=" + cfg.GOARCH,
		"GOOS=" + cfg.GOOS,
	}
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Env = append(cmd.Env, "HOME="+home)
	}
	if cfg.GOMODCACHE != "" {
		cmd.Env = append(cmd.Env, "GOMODCACHE="+cfg.GOMODCACHE)
	}
	if cfg.GOPROXY != "" {
		cmd.Env = append(cmd.Env, "GOPROXY="+cfg.GOPROXY)
	} else {
		cmd.Env = append(cmd.Env, "GOPROXY=off")
	}
	return cmd
}

func (e *Executor) mkdirTemp(ctx context.Context, pattern string) (string, func(), error) {
	dir, err := os.MkdirTemp("", pattern)
	if err != nil {
		return "", nil, err
	}
	cleanup := func() {
		if err := chmodR(dir, 0o777); err != nil {
			logctx.Errorln(ctx, err)
		}
		if err := os.RemoveAll(dir); err != nil {
			logctx.Errorln(ctx, err)
		}
	}
	return dir, cleanup, nil
}

func chmodR(root string, mode fs.FileMode) error {
	return filepath.WalkDir(root, func(p string, ent fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chmod(p, mode)
	})
}
