package want

import (
	"strings"

	"go.brendoncarroll.net/exp/singleflight"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/op/assertops"
	"wantbuild.io/want/src/internal/op/dagops"
	"wantbuild.io/want/src/internal/op/glfsops"
	"wantbuild.io/want/src/internal/op/goops"
	"wantbuild.io/want/src/internal/op/importops"
	"wantbuild.io/want/src/internal/op/qemuops"
	"wantbuild.io/want/src/internal/op/wantops"
	"wantbuild.io/want/src/internal/op/wasmops"
	"wantbuild.io/want/src/wantjob"
)

type executor struct {
	execs map[wantjob.OpName]wantjob.Executor
	sf    singleflight.Group[wantjob.TaskID, []byte]
}

type QEMUConfig = qemuops.Config

type ExecutorConfig struct {
	QEMU QEMUConfig

	GoDir string
}

func NewExecutor(cfg ExecutorConfig) wantjob.Executor {
	return newExecutor(cfg)
}

// newExecutor
// qemuDir is the qemu install dir
func newExecutor(cfg ExecutorConfig) *executor {

	qemuExec := qemuops.NewExecutor(cfg.QEMU)
	golangExec := goops.NewExecutor(cfg.GoDir)

	return &executor{
		execs: map[wantjob.OpName]wantjob.Executor{
			"glfs":   glfsops.Executor{},
			"import": importops.NewExecutor(),

			"dag":    dagops.Executor{},
			"assert": assertops.Executor{},
			"want": wantops.Executor{
				CompileOp: "want." + wantops.OpCompile,
				DAGExecOp: "dag." + dagops.OpExecAll,
			},

			"qemu":   qemuExec,
			"golang": golangExec,
			"wasm":   wasmops.NewExecutor(),
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
