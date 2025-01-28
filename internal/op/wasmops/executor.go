package wasmops

import (
	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/glfstasks"
	"wantbuild.io/want/lib/wantjob"
)

const (
	OpWASIp1         = wantjob.OpName("wasm.wasip1")
	OpNativeFunction = wantjob.OpName("wasm.nativeFunction")
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
			task, err := GetWASIp1Task(ctx, e.ag, src, x)
			if err != nil {
				return nil, err
			}
			return ComputeWASIp1(jc, src, *task)
		})
	case OpNativeFunction:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			task, err := GetGLFSTask(ctx, e.ag, src, x)
			if err != nil {
				return nil, err
			}
			return e.ComputeNative(jc, src, *task)
		})
	default:
		return nil, wantjob.NewErrUnknownOperator(x.Op)
	}
}
