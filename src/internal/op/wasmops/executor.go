package wasmops

import (
	"blobcache.io/blobcache/src/bcsdk"
	"blobcache.io/glfs"

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
	ag *glfs.Machine
}

func NewExecutor() *Executor {
	return &Executor{ag: glfs.NewMachine()}
}

func (e *Executor) Execute(jc wantjob.Ctx, src bcsdk.RO, x wantjob.Task) wantjob.Result {
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
		return *wantjob.Result_ErrExec(wantjob.NewErrUnknownOperator(x.Op))
	}
}
