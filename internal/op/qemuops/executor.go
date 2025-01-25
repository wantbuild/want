//go:build amd64 || arm64

package qemuops

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"
	"golang.org/x/sync/semaphore"

	"wantbuild.io/want/internal/glfstasks"
	"wantbuild.io/want/lib/wantjob"
)

var _ wantjob.Executor = &Executor{}

type Executor struct {
	// installDir contains the binaries needed to execute the VM operations
	installDir string
	memLimit   uint64
	memSem     *semaphore.Weighted
}

func NewExecutor(installDir string, memLimit uint64) *Executor {
	return &Executor{
		installDir: installDir,
		memLimit:   memLimit,
		memSem:     semaphore.NewWeighted(int64(memLimit)),
	}
}

func (e *Executor) Execute(jc wantjob.Ctx, src cadata.Getter, task wantjob.Task) ([]byte, error) {
	ctx := jc.Context
	switch task.Op {
	case OpAmd64MicroVMVirtioFS:
		return glfstasks.Exec(task.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			t, err := GetMicroVMTask(ctx, src, x)
			if err != nil {
				return nil, err
			}
			if t.Memory > e.memLimit {
				return nil, fmt.Errorf("task exceeds executor's memory limit %d > %d", t.Memory, e.memLimit)
			}
			if err := e.memSem.Acquire(ctx, int64(t.Memory)); err != nil {
				return nil, err
			}
			defer e.memSem.Release(int64(t.Memory))
			return e.RunMicroVM(jc, src, *t)
		})

	default:
		return nil, wantjob.NewErrUnknownOperator(task.Op)
	}
}

func (e *Executor) RunMicroVM(jc wantjob.Ctx, s cadata.Getter, t MicroVMTask) (*glfs.Ref, error) {
	vm, err := e.newVM(jc, s, vmConfig{
		NumCPUs: t.Cores,
		Memory:  t.Memory,
	}, kernelConfig{
		Init:     t.Init,
		InitArgs: t.Args,
	})
	if err != nil {
		return nil, err
	}
	defer vm.Close()
	jc.Infof("initialize rootfs: begin")
	if err := vm.init(jc.Context, t.Kernel, t.Root); err != nil {
		return nil, err
	}
	jc.Infof("initialize rootfs: done")
	jc.Infof("run vm")
	if err := vm.run(jc); err != nil {
		return nil, err
	}
	jc.Infof("import from vm fs: begin")
	output, err := vm.importPath(jc.Context, t.Output)
	if err != nil {
		return nil, err
	}
	jc.Infof("import from vm fs: end")
	if err := vm.Close(); err != nil {
		return nil, err
	}
	return output, nil
}

func (e *Executor) systemx86Cmd(args ...string) *exec.Cmd {
	cmdPath := filepath.Join(e.installDir, "qemu-system-x86_64")
	cmd := exec.Command(cmdPath, args...)
	cmd.Dir = e.installDir
	return cmd
}

func (e *Executor) virtiofsdCmd(args ...string) *exec.Cmd {
	cmdPath := filepath.Join(e.installDir, "virtiofsd")
	cmd := exec.Command(cmdPath, args...)
	cmd.Dir = e.installDir
	return cmd
}
