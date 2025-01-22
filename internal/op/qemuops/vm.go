//go:build amd64 || arm64

package qemuops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/glfsport"
	"wantbuild.io/want/lib/wantjob"
)

const (
	kernelFilename = "kernel"
	rootFSName     = "rootfs"
)

type vm struct {
	dir      string
	exp      glfsport.Exporter
	imp      glfsport.Importer
	viofsCmd *exec.Cmd
	qemuCmd  *exec.Cmd

	closed bool
}

type vmConfig struct {
	NumCPUs uint32
	Memory  uint64
}

type kernelConfig struct {
	Init     string
	InitArgs []string
}

func (e *Executor) newVM(jc wantjob.Ctx, src cadata.Getter, vmcfg vmConfig, kcfg kernelConfig) (*vm, error) {
	if vmcfg.NumCPUs == 0 {
		vmcfg.NumCPUs = uint32(runtime.NumCPU())
	}
	if vmcfg.Memory/1e6 < 1 {
		vmcfg.Memory = 512 * 1e6
	}
	dir, err := os.MkdirTemp("", "microvm-")
	if err != nil {
		return nil, err
	}
	jc.Infof("vm dir: %s", dir)

	vhostPath := filepath.Join(dir, "vhost.sock")
	rootFSPath := filepath.Join(dir, rootFSName)

	// viriofsd
	viofsCmd := func() *exec.Cmd {
		args := []string{
			fmt.Sprintf("--socket-path=%s", vhostPath),
			fmt.Sprintf("--shared-dir=%s", rootFSPath),
			//"--log-level=debug",
		}
		cmd := e.virtiofsdCmd(args...)
		cmd.Stdout = jc.Writer("virtiofsd/stdout")
		cmd.Stderr = jc.Writer("virtiofsd/stderr")
		return cmd
	}()
	// qemu
	qemuCmd := func() *exec.Cmd {
		kargs := kernelArgs{
			Console:        "hvc0",
			ClockSource:    "jiffies",
			IgnoreLogLevel: true,
			Reboot:         "t",
			Panic:          -1,
			Init:           kcfg.Init,
			InitArgs:       kcfg.InitArgs,
			RandomTrustCpu: "on",
		}
		kargs.VirtioFSRoot("myfs")

		args := []string{
			"-M", "microvm,x-option-roms=off,rtc=off,acpi=off",
			"-m", strconv.FormatUint(vmcfg.Memory/1e6, 10) + "M",
			"-smp", strconv.FormatUint(uint64(vmcfg.NumCPUs), 10),
			"-L", filepath.Join(e.installDir, "share"),

			//"-icount", "shift=auto",
			//"-rtc", "clock=vm,base=2000-01-01",

			"-kernel", filepath.Join(dir, kernelFilename),
			"-append", kargs.String(),

			"-display", "none",
			"-nodefaults",
			"-no-user-config",
			"-no-reboot",

			"-chardev", "stdio,id=virtiocon0",
			"-device", "virtio-serial-device",
			"-device", "virtconsole,chardev=virtiocon0",

			"-device", "virtio-rng-device",
		}

		// vhost socket
		args = append(args, "-chardev", fmt.Sprintf("socket,id=char0,path=%s", vhostPath))
		args = append(args, "-device", "vhost-user-fs-device,queue-size=1024,chardev=char0,tag=myfs")
		args = append(args, "-object", fmt.Sprintf("memory-backend-file,id=mem,size=%dM,mem-path=/dev/shm,share=on", vmcfg.Memory/1e6))
		args = append(args, "-numa", "node,memdev=mem")

		cmd := e.systemx86Cmd(args...)

		cmd.Stdout = jc.Writer("qemu/stdout")
		cmd.Stderr = jc.Writer("qemu/stderr")

		return cmd
	}()

	return &vm{
		dir: dir,
		exp: glfsport.Exporter{
			Cache: glfsport.NullCache{},
			Store: src,
			Dir:   dir,
		},
		imp: glfsport.Importer{
			Cache: glfsport.NullCache{},
			Store: jc.Dst,
			Dir:   dir,
		},
		viofsCmd: viofsCmd,
		qemuCmd:  qemuCmd,
	}, nil
}

func (vm *vm) init(ctx context.Context, kernel, rootfs glfs.Ref) error {
	if err := vm.exp.Export(ctx, kernel, kernelFilename); err != nil {
		return err
	}
	if err := vm.exp.Export(ctx, rootfs, "rootfs"); err != nil {
		return err
	}
	return nil
}

func (vm *vm) run(jc wantjob.Ctx) error {
	if err := vm.viofsCmd.Start(); err != nil {
		return err
	}
	if err := vm.awaitVhostSock(jc); err != nil {
		return err
	}

	jc.Debugf("args %v", vm.qemuCmd.Args)
	return vm.qemuCmd.Run()
}

func (vm *vm) awaitVhostSock(jc wantjob.Ctx) error {
	for i := 0; i < 10; i++ {
		_, err := os.Stat(vm.vhostPath())
		if os.IsNotExist(err) {
			jc.Infof("waiting for vhost.sock to come up")
			time.Sleep(100 * time.Millisecond)
		} else if err != nil {
			return err
		} else {
			jc.Infof("vhost.sock is up")
			return nil
		}
	}
	return fmt.Errorf("timedout waiting for %q", vm.vhostPath())
}

func (vm *vm) importPath(ctx context.Context, p string) (*glfs.Ref, error) {
	return vm.imp.Import(ctx, path.Join(rootFSName, p))
}

func (vm *vm) vhostPath() string {
	return filepath.Join(vm.dir, "vhost.sock")
}

func (vm *vm) Close() (retErr error) {
	if vm.dir == "" {
		return nil
	}
	if vm.closed {
		return nil
	}
	for _, closer := range []func() error{
		func() error {
			if vm.viofsCmd.Process != nil {
				if err := vm.viofsCmd.Process.Kill(); err != nil {
					return err
				}
				return vm.viofsCmd.Process.Release()
			}
			return nil
		},
		func() error {
			if vm.qemuCmd.Process != nil {
				if err := vm.qemuCmd.Process.Kill(); err != nil {
					return err
				}
				return vm.qemuCmd.Process.Release()
			}
			return nil
		},
		func() error {
			return os.RemoveAll(vm.dir)
		},
	} {
		if err := closer(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			retErr = errors.Join(retErr, err)
		}
	}
	vm.closed = true
	return retErr
}
