package wasmops

import (
	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/wantjob"
	"wantbuild.io/want/src/wantwasm"
)

const (
	OpWASIp1     = wantjob.OpName("wasip1")
	OpNativeGLFS = wantjob.OpName("nativeGLFS")
)

type (
	WASIp1Task     = wantwasm.WASIp1Task
	NativeGLFSTask = wantwasm.NativeGLFSTask
)

var _ wantjob.Executor = &Executor{}

type Executor struct {
	ag *glfs.Agent
}

func NewExecutor() *Executor {
	return &Executor{ag: glfs.NewAgent()}
}

func (e *Executor) Execute(jc wantjob.Ctx, src cadata.Getter, x wantjob.Task) ([]byte, error) {
	ctx := jc.Context
	switch x.Op {
	case OpWASIp1:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			task, err := wantwasm.GetWASIp1Task(ctx, e.ag, src, x)
			if err != nil {
				return nil, err
			}
			return ExecWASIp1(jc, src, *task)
		})
	case OpNativeGLFS:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			task, err := wantwasm.GetNativeTask(ctx, src, x)
			if err != nil {
				return nil, err
			}
			return e.ExecNativeGLFS(jc, src, *task)
		})
	default:
		return nil, wantjob.NewErrUnknownOperator(x.Op)
	}
}
