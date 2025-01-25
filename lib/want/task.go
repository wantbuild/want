package want

import (
	"strings"

	"go.brendoncarroll.net/exp/singleflight"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/op/assertops"
	"wantbuild.io/want/internal/op/dagops"
	"wantbuild.io/want/internal/op/glfsops"
	"wantbuild.io/want/internal/op/goops"
	"wantbuild.io/want/internal/op/importops"
	"wantbuild.io/want/internal/op/qemuops"
	"wantbuild.io/want/internal/op/wantops"
	"wantbuild.io/want/lib/wantjob"
)

type executor struct {
	execs map[wantjob.OpName]wantjob.Executor
	sf    singleflight.Group[wantjob.TaskID, []byte]
}

type executorConfig struct {
	QEMUDir    string
	QEMUMemory uint64

	GoDir string
}

// newExecutor
// qemuDir is the qemu install dir
func newExecutor(cfg executorConfig) *executor {

	qemuExec := qemuops.NewExecutor(cfg.QEMUDir, cfg.QEMUMemory)
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
