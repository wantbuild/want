package want

import (
	"fmt"
	"os"
	"strings"

	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/op/assertops"
	"wantbuild.io/want/src/internal/op/dagops"
	"wantbuild.io/want/src/internal/op/glfsops"
	"wantbuild.io/want/src/internal/op/goops"
	"wantbuild.io/want/src/internal/op/importops"
	"wantbuild.io/want/src/internal/op/qemuops"
	"wantbuild.io/want/src/internal/op/wantops"
	"wantbuild.io/want/src/internal/op/wasmops"
	"wantbuild.io/want/src/internal/wantsetup"
	"wantbuild.io/want/src/wantjob"
)

type executorFactory = func(jc wantjob.Ctx) (wantjob.Executor, error)

type executor struct {
	execs map[wantjob.OpName]wantjob.Executor
	setup map[wantjob.OpName]executorFactory

	setupOg onceGroup[string, wantjob.Executor]
}

type QEMUConfig = qemuops.Config

type ExecutorConfig struct {
	QEMU QEMUConfig

	GoRoot  string
	GoState string
}

func NewExecutor(cfg ExecutorConfig) wantjob.Executor {
	return newExecutor(cfg)
}

// newExecutor
// qemuDir is the qemu install dir
func newExecutor(cfg ExecutorConfig) *executor {
	return &executor{
		execs: map[wantjob.OpName]wantjob.Executor{
			"glfs":   glfsops.Executor{},
			"import": importops.NewExecutor(),

			"dag":    dagops.Executor{},
			"assert": assertops.Executor{},
			"want": wantops.Executor{
				CompileOp: "want." + wantops.OpCompile,
				DAGExecOp: "dag." + dagops.OpExecLast,
			},

			"wasm": wasmops.NewExecutor(),
		},
		setup: map[wantjob.OpName]func(jc wantjob.Ctx) (wantjob.Executor, error){
			"qemu": func(jc wantjob.Ctx) (wantjob.Executor, error) {
				if err := install(jc, qemuops.InstallSnippet(), cfg.QEMU.InstallDir); err != nil {
					return nil, err
				}
				return qemuops.NewExecutor(cfg.QEMU), nil
			},
			"golang": func(jc wantjob.Ctx) (wantjob.Executor, error) {
				if err := install(jc, goops.InstallSnippet(), cfg.GoRoot); err != nil {
					return nil, err
				}
				if err := os.MkdirAll(cfg.GoState, 0o755); err != nil {
					return nil, err
				}
				return goops.NewExecutor(cfg.GoRoot, cfg.GoState), nil
			},
		},
	}
}

func (e *executor) Execute(jc wantjob.Ctx, src cadata.Getter, task wantjob.Task) wantjob.Result {
	parts := strings.SplitN(string(task.Op), ".", 2)
	execName := wantjob.OpName(parts[0])

	e2, exists := e.execs[execName]
	if !exists {
		var err error
		if e2, err = e.setupOg.Do(parts[0], func() (wantjob.Executor, error) {
			setup, exists := e.setup[execName]
			if !exists {
				return nil, wantjob.ErrOpNotFound{Op: task.Op}
			}
			exec, err := setup(jc)
			if err != nil {
				return nil, fmt.Errorf("setting up exec for %v: %w", task.Op, err)
			}
			return exec, nil
		}); err != nil {
			return *wantjob.Result_ErrInternal(err)
		}
	}
	return e2.Execute(jc, src, wantjob.Task{
		Op:    wantjob.OpName(parts[1]),
		Input: task.Input,
	})
}

func install(jc wantjob.Ctx, snippet string, dstPath string) error {
	if _, err := os.Stat(dstPath); err == nil {
		// TODO: better way to verify the integrity of the install.
		return nil
	}
	return wantsetup.Install(jc.Context, jc.System, dstPath, snippet)
}
