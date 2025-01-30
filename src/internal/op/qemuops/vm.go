//go:build amd64 || arm64

package qemuops

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"time"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/glfsport"
	"wantbuild.io/want/src/wantjob"
)

const (
	kernelFilename = "kernel"
	rootFSName     = "rootfs"
)

func (e *Executor) newVM_VirtioFS(jc wantjob.Ctx, src cadata.Getter, dir string, vmcfg vmConfig) (*vm_VirtioFS, error) {
	vhostPath := filepath.Join(dir, "vhost.sock")
	rootFSPath := filepath.Join(dir, rootFSName)

	uid := os.Getuid()
	gid := os.Getgid()
	const maxIdRange = 1 << 16
	// viriofsd
	viofsCmd := func() *exec.Cmd {
		args := []string{
			fmt.Sprintf("--socket-path=%s", vhostPath),
			fmt.Sprintf("--shared-dir=%s", rootFSPath),
			"--cache=always",
			"--sandbox=namespace",

			fmt.Sprintf("--translate-uid=squash-host:0:0:%d", maxIdRange),
			fmt.Sprintf("--translate-gid=squash-host:0:0:%d", maxIdRange),
			fmt.Sprintf("--translate-uid=squash-guest:0:%d:%d", uid, maxIdRange),
			fmt.Sprintf("--translate-gid=squash-guest:0:%d:%d", gid, maxIdRange),
			//"--log-level=debug",
		}
		cmd := e.virtiofsdCmd(args...)
		cmd.Stdout = jc.Writer("virtiofsd/stdout")
		cmd.Stderr = jc.Writer("virtiofsd/stderr")
		return cmd
	}()
	vmcfg.CharDevs["char0"] = chardevConfig{
		Backend: "socket",
		Props: map[string]string{
			"path": vhostPath,
		},
	}
	vmcfg.Objects["mem0"] = objectConfig{
		Type: "memory-backend-file",
		Props: map[string]string{
			"size":     fmt.Sprintf("%dM", vmcfg.Memory/1e6),
			"mem-path": "/dev/shm",
			"share":    "on",
		},
	}
	vmcfg.Numa = []numaConfig{
		{Type: "node", MemDev: "mem0"},
	}
	vmcfg.Devices = append(vmcfg.Devices, deviceConfig{
		Type: "vhost-user-fs-device",
		Props: map[string]string{
			"queue-size": "1024",
			"chardev":    "char0",
			"tag":        "myfs",
		},
	})

	vm, err := e.newVM(jc, src, dir, vmcfg)
	if err != nil {
		return nil, err
	}
	return &vm_VirtioFS{
		vm:       vm,
		viofsCmd: viofsCmd,
	}, nil
}

// vm_VirtioFS is a VM with a virtiofs root
// in addition to the QEMU VM it manages a virtiofsd instance
// sharing a directory on the host.
type vm_VirtioFS struct {
	*vm

	viofsCmd *exec.Cmd
}

func (vm *vm_VirtioFS) init(ctx context.Context, kernel, rootfs glfs.Ref) error {
	if err := vm.exp.Export(ctx, rootfs, "rootfs"); err != nil {
		return err
	}
	return vm.vm.init(ctx, kernel)
}

func (vm *vm_VirtioFS) run(jc wantjob.Ctx) error {
	if err := vm.viofsCmd.Start(); err != nil {
		return err
	}
	if err := vm.awaitVhostSock(jc); err != nil {
		return err
	}
	return vm.vm.run(jc)
}

func (vm *vm_VirtioFS) awaitVhostSock(jc wantjob.Ctx) error {
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

func (vm *vm_VirtioFS) vhostPath() string {
	return filepath.Join(vm.dir, "vhost.sock")
}

func (vm *vm_VirtioFS) importPath(ctx context.Context, p string) (*glfs.Ref, error) {
	return vm.vm.imp.Import(ctx, path.Join(rootFSName, p))
}

func (vm *vm_VirtioFS) Close() error {
	if vm.viofsCmd.Process != nil {
		if err := vm.viofsCmd.Process.Kill(); err != nil {
			return err
		}
		return vm.viofsCmd.Process.Release()
	}
	return vm.vm.Close()
}

type vm struct {
	dir string
	exp glfsport.Exporter
	imp glfsport.Importer

	qemuCmd *exec.Cmd

	closed bool
}

func (e *Executor) newVM(jc wantjob.Ctx, src cadata.Getter, dir string, vmcfg vmConfig) (*vm, error) {
	if vmcfg.NumCPUs == 0 {
		vmcfg.NumCPUs = uint32(runtime.NumCPU())
	}
	if vmcfg.Memory/1e6 < 1 {
		vmcfg.Memory = 512 * 1e6
	}
	jc.Infof("vm dir: %s", dir)

	// qemu
	qemuCmd := func() *exec.Cmd {
		args := []string{
			"-M", "microvm,x-option-roms=off,rtc=off,acpi=off",
			"-m", strconv.FormatUint(vmcfg.Memory/1e6, 10) + "M",
			"-smp", strconv.FormatUint(uint64(vmcfg.NumCPUs), 10),
			"-L", filepath.Join(e.cfg.InstallDir, "share"),

			"-kernel", filepath.Join(dir, kernelFilename),
			"-append", vmcfg.AppendKernelArgs,

			"-display", "none",
			"-nodefaults",
			"-no-user-config",
			"-no-reboot",

			"-chardev", "stdio,id=virtiocon0",
			"-device", "virtio-serial-device",
			"-device", "virtconsole,chardev=virtiocon0",
		}
		args = vmcfg.DeviceArgs(args)

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
		qemuCmd: qemuCmd,
	}, nil
}

func (vm *vm) init(ctx context.Context, kernel glfs.Ref) error {
	if err := vm.exp.Export(ctx, kernel, kernelFilename); err != nil {
		return err
	}
	return nil
}

func (vm *vm) run(jc wantjob.Ctx) error {
	jc.Debugf("args %v", vm.qemuCmd.Args)
	return vm.qemuCmd.Run()
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

func appendBackendArgs(args []string, ty string, id string, props map[string]string) []string {
	s := ty + ",id=" + id
	for k, v := range sortedKeys(props) {
		s += "," + k + "=" + v
	}
	args = append(args, s)
	return args
}

type objectConfig struct {
	Type  string
	Props map[string]string
}

func (oc objectConfig) appendArgs(args []string, id string) []string {
	args = append(args, "-object")
	return appendBackendArgs(args, oc.Type, id, oc.Props)
}

type numaConfig struct {
	Type string

	MemDev string
}

func (nc numaConfig) appendArgs(args []string) []string {
	return append(args, "-numa", nc.Type+",memdev="+nc.MemDev)
}

type chardevConfig struct {
	Backend string
	Props   map[string]string
}

func (c chardevConfig) appendArgs(args []string, id string) []string {
	args = append(args, "-chardev")
	args = appendBackendArgs(args, c.Backend, id, c.Props)
	return args
}

type netdevConfig struct {
	Backend string
	Props   map[string]string
}

func (c netdevConfig) appendArgs(args []string, id string) []string {
	args = append(args, "-netdev")
	args = appendBackendArgs(args, c.Backend, id, c.Props)
	return args
}

type deviceConfig struct {
	Type string

	Props map[string]string
}

func (cfg deviceConfig) appendArgs(args []string) []string {
	args = append(args, "-device")
	s := cfg.Type
	for k, v := range sortedKeys(cfg.Props) {
		s += "," + k + "=" + v
	}
	args = append(args, s)
	return args
}

type vmConfig struct {
	NumCPUs uint32
	Memory  uint64

	AppendKernelArgs string

	CharDevs map[string]chardevConfig
	NetDevs  map[string]netdevConfig
	Objects  map[string]objectConfig
	Numa     []numaConfig

	Devices []deviceConfig
}

func (vc vmConfig) Args(args []string) []string {
	if vc.NumCPUs > 0 {
		panic(vc.NumCPUs)
	}
	if vc.Memory > 0 {
		panic(vc.Memory)
	}
	args = vc.DeviceArgs(args)
	return args
}

func (vc vmConfig) DeviceArgs(args []string) []string {
	// backends
	for id, dev := range vc.CharDevs {
		args = dev.appendArgs(args, id)
	}
	for id, dev := range vc.NetDevs {
		args = dev.appendArgs(args, id)
	}
	for id, dev := range vc.Objects {
		args = dev.appendArgs(args, id)
	}
	for _, dev := range vc.Numa {
		args = dev.appendArgs(args)
	}

	// frontends
	for _, dev := range vc.Devices {
		args = dev.appendArgs(args)
	}
	return args
}

func sortedKeys[V any](m map[string]V) iter.Seq2[string, V] {
	ks := slices.Collect(maps.Keys(m))
	slices.Sort(ks)
	return func(yield func(string, V) bool) {
		for _, k := range ks {
			if !yield(k, m[k]) {
				break
			}
		}
	}
}
