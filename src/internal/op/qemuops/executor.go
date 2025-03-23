//go:build amd64 || arm64

package qemuops

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"blobcache.io/glfs"
	"go.brendoncarroll.net/state/cadata"
	"golang.org/x/sync/semaphore"

	"wantbuild.io/want/src/internal/glfscpio"
	"wantbuild.io/want/src/internal/glfsport"
	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/wantjob"
	"wantbuild.io/want/src/wantqemu"
)

const (
	OpAmd64MicroVM = wantjob.OpName("amd64_microvm")
)

type MicroVMTask = wantqemu.MicroVMTask

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

func (e *Executor) Execute(jc wantjob.Ctx, src cadata.Getter, task wantjob.Task) wantjob.Result {
	ctx := jc.Context
	switch task.Op {
	case OpAmd64MicroVM:
		return glfstasks.Exec(task.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			t, err := wantqemu.GetMicroVMTask(ctx, src, x)
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
			return e.amd64MicroVM(jc, src, *t)
		})

	default:
		return *wantjob.Result_ErrExec(wantjob.NewErrUnknownOperator(task.Op))
	}
}

func (e *Executor) amd64MicroVM(jc wantjob.Ctx, s cadata.Getter, t MicroVMTask) (*glfs.Ref, error) {
	dir, err := os.MkdirTemp("", "microvm-")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			jc.Errorf("cleaning up vm dir %v: %w", dir, err)
		}
	}()

	// begin default vmConfig
	vmCfg := vmConfig{
		NumCPUs: t.Cores,
		Memory:  t.Memory,

		CharDevs: map[string]chardevConfig{
			"virtiocon0": {
				Backend: "stdio",
			},
		},
		NetDevs: map[string]netdevConfig{},
		Objects: map[string]objectConfig{},
	}
	vmCfg.addDevice(deviceConfig{
		Type: "virtio-serial-device",
	})
	vmCfg.addDevice(deviceConfig{
		Type: "virtconsole",
		Props: map[string]string{
			"chardev": "virtiocon0",
		},
	})
	// end default vmConfig
	if t.KernelArgs != "" {
		vmCfg.AppendKernelArgs = t.KernelArgs
	}
	// virtiofs
	if len(t.VirtioFS) > 1 {
		return nil, fmt.Errorf("multiple virtiofs are not yet supported")
	}
	vfsds := make(map[string]*virtioFSd)
	for k, spec := range t.VirtioFS {
		if strings.Contains(k, "..") {
			return nil, fmt.Errorf("invalid name %v", k)
		}
		rootFSPath := filepath.Join(dir, k+"-root")
		vhostPath := filepath.Join(dir, k+"-vhost.sock")
		configAddVirtioFS(&vmCfg, vhostPath, k)
		stdout := jc.Writer("virtiofsd/stdout")
		stderr := jc.Writer("virtiofsd/stderr")
		vfsd := e.newVirtioFSd(rootFSPath, vhostPath, stdout, stderr)
		defer vfsd.Close()
		vfsds[k] = vfsd

		done := jc.InfoSpan("setup virtiofs " + k)
		if err := vfsd.Export(jc.Context, s, "", spec.Root); err != nil {
			return nil, err
		}
		if err := vfsd.Start(); err != nil {
			return nil, err
		}
		if err := vfsd.awaitVhostSock(jc); err != nil {
			return nil, err
		}
		done()
	}
	exp := glfsport.Exporter{
		Dir:   dir,
		Cache: glfsport.NullCache{},
		Store: s,
	}
	// kernel
	df := jc.InfoSpan("setup kernel")
	if err := exp.Export(jc.Context, t.Kernel, kernelFilename); err != nil {
		return nil, err
	}
	df()
	// initrd
	if t.Initrd != nil {
		vmCfg.Initrd = true
		df := jc.InfoSpan("setup initrd")
		if err := exportInitrd(jc.Context, s, dir, *t.Initrd); err != nil {
			return nil, err
		}
		df()
	}

	vm := e.newVM(jc, dir, vmCfg)
	defer vm.Close()
	jc.Infof("run vm")
	if err := vm.run(jc); err != nil {
		return nil, err
	}
	if err := vm.Close(); err != nil {
		return nil, err
	}
	switch {
	case t.Output.VirtioFS != nil:
		spec := *t.Output.VirtioFS
		vfsd := vfsds[spec.ID]
		if vfsd == nil {
			return nil, fmt.Errorf("no such virtiofs id=%v", vfsd)
		}
		defer jc.InfoSpan("importing from virtiofs")()
		return vfsd.Import(jc.Context, jc.Dst, spec.Query)
	default:
		return nil, ErrInvalidOutputSpec{t.Output}
	}
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

func exportInitrd(ctx context.Context, s cadata.Getter, dir string, initrdRef glfs.Ref) error {
	f, err := os.OpenFile(filepath.Join(dir, initrdFilename), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if initrdRef.Type == glfs.TypeTree {
		// if it's a tree, then we need to
		if err := glfscpio.Write(ctx, s, initrdRef, f); err != nil {
			return err
		}
	} else {
		// if it's a blob, assume it's an image that can be passed directly to QEMU
		r, err := glfs.GetBlob(ctx, s, initrdRef)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, r); err != nil {
			return err
		}
	}
	return f.Close()
}

type ErrInvalidOutputSpec struct {
	Spec wantqemu.Output
}

func (e ErrInvalidOutputSpec) Error() string {
	return fmt.Sprintf("invalid output spec: %v", e.Spec)
}
