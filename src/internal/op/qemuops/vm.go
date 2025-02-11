//go:build amd64 || arm64

package qemuops

import (
	"errors"
	"fmt"
	"iter"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"

	"wantbuild.io/want/src/wantjob"
)

const (
	kernelFilename = "kernel"
	initrdFilename = "initrd"
)

type vm struct {
	dir string

	qemuCmd *exec.Cmd
	closed  bool
}

func (e *Executor) newVM(jc wantjob.Ctx, dir string, vmcfg vmConfig) *vm {
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

			"-display", "none",
			"-nodefaults",
			"-no-user-config",
			"-no-reboot",

			"-kernel", filepath.Join(dir, kernelFilename),
		}
		add := func(xs ...string) {
			args = append(args, xs...)
		}
		if vmcfg.AppendKernelArgs != "" {
			add("-append", vmcfg.AppendKernelArgs)
		}
		if vmcfg.Initrd {
			add("-initrd", filepath.Join(dir, initrdFilename))
		}
		args = vmcfg.DeviceArgs(args)

		cmd := e.systemx86Cmd(args...)
		cmd.Stdout = jc.Writer("qemu/stdout")
		cmd.Stderr = jc.Writer("qemu/stderr")
		return cmd
	}()

	return &vm{
		dir:     dir,
		qemuCmd: qemuCmd,
	}
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

// vmConfig is a structured form of the command line configuration that
// will be passed to QEMU.
type vmConfig struct {
	NumCPUs uint32
	Memory  uint64

	AppendKernelArgs string
	Initrd           bool

	CharDevs map[string]chardevConfig
	NetDevs  map[string]netdevConfig
	Objects  map[string]objectConfig
	Numa     []numaConfig

	Devices []deviceConfig
}

func (vc *vmConfig) addDevice(dc deviceConfig) {
	vc.Devices = append(vc.Devices, dc)
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

func configAddVirtioFS(vmcfg *vmConfig, vhostPath string, tag string) {
	charDevID := "vfs_" + tag
	vmcfg.CharDevs[charDevID] = chardevConfig{
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
	vmcfg.Numa = append(vmcfg.Numa, numaConfig{
		Type: "node", MemDev: "mem0",
	})
	vmcfg.Devices = append(vmcfg.Devices, deviceConfig{
		Type: "vhost-user-fs-device",
		Props: map[string]string{
			"queue-size": "1024",
			"chardev":    charDevID,
			"tag":        tag,
		},
	})
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
