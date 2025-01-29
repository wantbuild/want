//go:build amd64 || arm64

package qemuops

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"
	"golang.org/x/sync/semaphore"

	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/wantjob"
)

var _ wantjob.Executor = &Executor{}

type Executor struct {
	cfg    Config
	memSem *semaphore.Weighted
}

// Config has configuration for the executor
type Config struct {
	// InstallDir contains the binaries needed to execute the VM operations
	InstallDir string
	MemLimit   int64
}

func NewExecutor(cfg Config) *Executor {
	return &Executor{
		cfg:    cfg,
		memSem: semaphore.NewWeighted(cfg.MemLimit),
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
			if t.Memory > uint64(e.cfg.MemLimit) {
				return nil, fmt.Errorf("task exceeds executor's memory limit %d > %d", t.Memory, e.cfg.MemLimit)
			}
			if err := e.memSem.Acquire(ctx, int64(t.Memory)); err != nil {
				return nil, err
			}
			defer e.memSem.Release(int64(t.Memory))
			return e.amd64MicroVMVirtiofs(jc, src, *t)
		})

	default:
		return nil, wantjob.NewErrUnknownOperator(task.Op)
	}
}

func (e *Executor) amd64MicroVMVirtiofs(jc wantjob.Ctx, s cadata.Getter, t MicroVMTask) (*glfs.Ref, error) {
	dir, err := os.MkdirTemp("", "microvm-")
	if err != nil {
		return nil, err
	}

	kargs := kernelArgs{
		Console:        "hvc0",
		ClockSource:    "jiffies",
		IgnoreLogLevel: true,
		Reboot:         "t",
		Panic:          -1,
		Init:           t.Init,
		InitArgs:       t.Args,
		RandomTrustCpu: "on",
	}.VirtioFSRoot("myfs")

	vm, err := e.newVM_VirtioFS(jc, s, dir, vmConfig{
		NumCPUs:          t.Cores,
		Memory:           t.Memory,
		AppendKernelArgs: kargs.String(),

		CharDevs: make(map[string]chardevConfig),
		NetDevs:  make(map[string]netdevConfig),
		Objects:  make(map[string]objectConfig),
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
	cmdPath := filepath.Join(e.cfg.InstallDir, "qemu-system-x86_64")
	cmd := exec.Command(cmdPath, args...)
	cmd.Dir = e.cfg.InstallDir
	return cmd
}

func (e *Executor) virtiofsdCmd(args ...string) *exec.Cmd {
	cmdPath := filepath.Join(e.cfg.InstallDir, "virtiofsd")
	cmd := exec.Command(cmdPath, args...)
	cmd.Dir = e.cfg.InstallDir
	return cmd
}
