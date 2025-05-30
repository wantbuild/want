//go:build amd64 || arm64

package qemuops

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
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
	"wantbuild.io/want/src/internal/streammux"
	"wantbuild.io/want/src/wantjob"
	"wantbuild.io/want/src/wantjob/wanthttp"
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
		inputRef, err := glfstasks.ParseGLFSRef(task.Input)
		if err != nil {
			return *wantjob.Result_ErrExec(err)
		}
		t, err := wantqemu.GetMicroVMTask(ctx, src, *inputRef)
		if err != nil {
			return *wantjob.Result_ErrExec(err)
		}
		if t.Memory > uint64(e.cfg.MemLimit) {
			return *wantjob.Result_ErrExec(fmt.Errorf("task exceeds executor's memory limit %d > %d", t.Memory, e.cfg.MemLimit))
		}
		if err := e.memSem.Acquire(ctx, int64(t.Memory)); err != nil {
			return *wantjob.Result_ErrExec(err)
		}
		defer e.memSem.Release(int64(t.Memory))
		res, err := e.amd64MicroVM(jc, src, *t)
		if err != nil {
			return *wantjob.Result_ErrExec(err)
		}
		return *res
	default:
		return *wantjob.Result_ErrExec(wantjob.NewErrUnknownOperator(task.Op))
	}
}

func (e *Executor) amd64MicroVM(jc wantjob.Ctx, s cadata.Getter, t MicroVMTask) (*wantjob.Result, error) {
	dir, err := os.MkdirTemp("", "microvm-")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			jc.Errorf("cleaning up vm dir %v: %w", dir, err)
		}
	}()

	mctx := &microVMTaskCtx{
		jc:      jc,
		dir:     dir,
		hsrv:    wanthttp.NewServer(jc.System),
		vfsds:   make(map[string]*virtioFSd),
		sockets: make(map[string]net.Listener),
	}

	// begin default vmConfig
	vmCfg := vmConfig{
		NumCPUs: t.Cores,
		Memory:  t.Memory,

		CharDevs: map[string]chardevConfig{},
		NetDevs:  map[string]netdevConfig{},
		Objects:  map[string]objectConfig{},
	}
	mctx.vmCfg = &vmCfg
	// end default vmConfig
	if t.KernelArgs != "" {
		vmCfg.AppendKernelArgs = t.KernelArgs
	}

	// serial ports
	if len(t.SerialPorts) > 0 {
		vmCfg.addDevice(deviceConfig{
			Type: "virtio-serial-device",
		})
	}
	for _, spec := range t.SerialPorts {
		if err := e.addSerialPort(mctx, spec); err != nil {
			return nil, err
		}
	}
	defer func() {
		for _, l := range mctx.sockets {
			l.Close()
		}
	}()

	// virtiofs
	if len(t.VirtioFS) > 1 {
		return nil, fmt.Errorf("multiple virtiofs are not yet supported")
	}
	for k, spec := range t.VirtioFS {
		if err := e.addVirtioFS(jc, s, mctx, k, spec); err != nil {
			return nil, err
		}
	}
	defer func() {
		for _, vfsd := range mctx.vfsds {
			vfsd.Close()
		}
	}()

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

	// input
	mctx.setInput(s, t.Input.Root)

	// create and run vm
	vm := e.newVM(jc, dir, vmCfg)
	defer vm.Close()
	jc.Infof("run vm")
	if err := vm.run(jc); err != nil {
		return nil, err
	}
	if err := vm.Close(); err != nil {
		return nil, err
	}

	// get output
	switch {
	case t.Output.JobOutput != nil:
		if mctx.hsrv == nil {
			return nil, fmt.Errorf("no want API running")
		}
		res := mctx.hsrv.GetResult()
		if res == nil {
			return nil, fmt.Errorf("no result")
		}
		return res, nil
	case t.Output.VirtioFS != nil:
		spec := *t.Output.VirtioFS
		vfsd := mctx.vfsds[spec.ID]
		if vfsd == nil {
			return nil, fmt.Errorf("no such virtiofs id=%v", vfsd)
		}
		defer jc.InfoSpan("importing from virtiofs")()
		ref, err := vfsd.Import(jc.Context, jc.Dst, spec.Query)
		if err != nil {
			return nil, err
		}
		return glfstasks.Success(*ref), nil
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

func (e *Executor) addSerialPort(mctx *microVMTaskCtx, spec wantqemu.SerialSpec) error {
	vmCfg := mctx.vmCfg
	switch {
	case spec.WantHTTP != nil:
		sockPath := filepath.Join(mctx.dir, "want.sock")
		l, err := net.Listen("unix", sockPath)
		if err != nil {
			return err
		}
		mctx.sockets["wanthttp"] = l
		go func() {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
			mux := streammux.New(conn)
			lis := &streammux.Listener{Mux: mux, Context: mctx.jc.Context}
			if err := http.Serve(lis, mctx.hsrv); err != nil {
				mctx.jc.Errorf("http.Serve: %v", err)
			}
		}()
		vmCfg.CharDevs["wanthttp"] = chardevConfig{
			Backend: "socket",
			Props: map[string]string{
				"path": sockPath,
			},
		}
		vmCfg.addDevice(deviceConfig{
			Type: "virtserialport",
			Props: map[string]string{
				"chardev": "wanthttp",
			},
		})
	case spec.Console != nil:
		vmCfg.CharDevs["console"] = chardevConfig{
			Backend: "stdio",
		}
		vmCfg.addDevice(deviceConfig{
			Type: "virtconsole",
			Props: map[string]string{
				"chardev": "console",
			},
		})
	default:
		return fmt.Errorf("empty serial port spec")
	}
	return nil
}

func (e *Executor) addVirtioFS(jc wantjob.Ctx, s cadata.Getter, mctx *microVMTaskCtx, k string, spec wantqemu.VirtioFSSpec) error {
	vmCfg := mctx.vmCfg
	if strings.Contains(k, "..") {
		return fmt.Errorf("invalid name %v", k)
	}
	rootFSPath := filepath.Join(mctx.dir, k+"-root")
	vhostPath := filepath.Join(mctx.dir, k+"-vhost.sock")
	configAddVirtioFS(vmCfg, vhostPath, k)
	stdout := jc.Writer("virtiofsd/stdout")
	stderr := jc.Writer("virtiofsd/stderr")
	vfsd := e.newVirtioFSd(rootFSPath, vhostPath, stdout, stderr)
	mctx.vfsds[k] = vfsd

	done := jc.InfoSpan("setup virtiofs " + k)
	if err := vfsd.Export(jc.Context, s, "", spec.Root); err != nil {
		return err
	}
	if err := vfsd.Start(); err != nil {
		return err
	}
	if err := vfsd.awaitVhostSock(jc); err != nil {
		return err
	}
	done()
	return nil
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

// microVMTaskCtx holds all the state for a microVM task
type microVMTaskCtx struct {
	jc      wantjob.Ctx
	dir     string
	vfsds   map[string]*virtioFSd
	sockets map[string]net.Listener
	hsrv    *wanthttp.Server
	vmCfg   *vmConfig
	vm      *vm
}

func (mctx *microVMTaskCtx) setupHTTP(jsys wantjob.System) {
	mctx.hsrv = wanthttp.NewServer(jsys)
}

func (mctx *microVMTaskCtx) setInput(src cadata.Getter, x []byte) {
	mctx.hsrv.SetInput(src, x)
}
