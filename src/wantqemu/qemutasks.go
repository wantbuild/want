package wantqemu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"slices"

	"blobcache.io/glfs"
	"go.brendoncarroll.net/state/cadata"
	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/wantcfg"
	"wantbuild.io/want/src/wantjob"
)

// MicroVMTask is an Amd64 Linux MicroVM Task
type MicroVMTask struct {
	Cores uint32
	// Memory is the memory in bytes
	Memory uint64

	// Kernel is the linux kernel image used to boot the VM.
	Kernel     glfs.Ref
	KernelArgs string
	Initrd     *glfs.Ref

	SerialPorts []SerialSpec
	VirtioFS    map[string]VirtioFSSpec

	Input  Input
	Output Output
}

func (t MicroVMTask) Validate() error {
	if t.Output.VirtioFS != nil {
		k := t.Output.VirtioFS.ID
		if _, exists := t.VirtioFS[k]; !exists {
			return fmt.Errorf("output refers to virtiofs (id=%s) which does not exist", k)
		}
	}
	if t.Output.JobOutput != nil {
		if !slices.ContainsFunc(t.SerialPorts, func(s SerialSpec) bool {
			return s.WantHTTP != nil
		}) {
			return fmt.Errorf("output.joboutput requires that the want API is available")
		}
	}
	return nil
}

type SerialSpec struct {
	WantHTTP *struct{} `json:"wanthttp,omitempty"`
	Console  *struct{} `jaon:"console,omitempty"`
}

type VirtioFSSpec struct {
	// Root is the initial data in the filesystem
	Root glfs.Ref `json:"-"`
	// Writable if the filesystem should be made writable.
	Writeable bool `json:"writeable"`
}

type VirtioFSOutput struct {
	// ID is the id of the virtiofs filesystem
	ID    string          `json:"id"`
	Query wantcfg.PathSet `json:"query"`
}

type Output struct {
	// VirtioFS will read the output from a virtiofs filesystem
	VirtioFS  *VirtioFSOutput `json:"virtiofs,omitempty"`
	JobOutput *struct{}       `json:"job,omitempty"`
}

// Input describes where to get the task input from.
type Input struct {
	Schema wantjob.Schema `json:"schema"`
	Root   []byte         `json:"root"`
}

func GrabVirtioFS(fsid string, q wantcfg.PathSet) Output {
	return Output{VirtioFS: &VirtioFSOutput{ID: fsid, Query: q}}
}

// microVMConfig is the config file for a MicroVMTask
type microVMConfig struct {
	Cores       uint32                  `json:"cores"`
	Memory      uint64                  `json:"memory"`
	KernelArgs  string                  `json:"kernel_args"`
	SerialPorts []SerialSpec            `json:"serial_ports"`
	VirtioFS    map[string]VirtioFSSpec `json:"virtiofs"`
	Input       Input                   `json:"input"`
	Output      Output                  `json:"output"`
}

func PostMicroVMTask(ctx context.Context, s cadata.PostExister, x MicroVMTask) (*glfs.Ref, error) {
	ag := glfs.NewAgent()
	configData, err := json.Marshal(microVMConfig{
		Cores:       x.Cores,
		Memory:      x.Memory,
		KernelArgs:  x.KernelArgs,
		SerialPorts: x.SerialPorts,
		VirtioFS:    x.VirtioFS,
		Input:       x.Input,
		Output:      x.Output,
	})
	if err != nil {
		return nil, err
	}
	cRef, err := ag.PostBlob(ctx, s, bytes.NewReader(configData))
	if err != nil {
		return nil, err
	}
	ents := []glfs.TreeEntry{
		{Name: "vm.json", Ref: *cRef},
		{Name: "kernel", Ref: x.Kernel},
	}
	if x.Initrd != nil {
		ents = append(ents, glfs.TreeEntry{Name: "initrd", Ref: *x.Initrd})
	}
	if x.Input.Schema == wantjob.Schema_GLFS {
		ref, err := glfstasks.ParseGLFSRef(x.Input.Root)
		if err != nil {
			return nil, fmt.Errorf("invalid input for schema: %w", err)
		}
		ents = append(ents, glfs.TreeEntry{Name: "input", Ref: *ref})
	}
	for name, vfs := range x.VirtioFS {
		ents = append(ents, glfs.TreeEntry{Name: path.Join("virtiofs", name), FileMode: 0o777, Ref: vfs.Root})
	}
	return ag.PostTreeSlice(ctx, s, ents)
}

func GetMicroVMTask(ctx context.Context, s cadata.Getter, x glfs.Ref) (*MicroVMTask, error) {
	// config
	cfg, err := glfstasks.GetJSONAt[microVMConfig](ctx, s, x, "vm.json")
	if err != nil {
		return nil, err
	}
	// kernel
	kRef, err := glfs.GetAtPath(ctx, s, x, "kernel")
	if err != nil {
		return nil, err
	}
	// initrd
	initrd, err := glfs.GetAtPath(ctx, s, x, "initrd")
	if err != nil && !glfs.IsErrNoEnt(err) {
		return nil, err
	}
	// virtiofs
	if len(cfg.VirtioFS) > 0 {
		vfsdir, err := glfs.GetAtPath(ctx, s, x, "virtiofs")
		if err != nil {
			return nil, err
		}
		vfsm, err := glfstasks.GetMap(ctx, s, *vfsdir, func(ctx context.Context, g cadata.Getter, r glfs.Ref) (*glfs.Ref, error) {
			return &r, nil
		})
		if err != nil {
			return nil, err
		}
		for k := range vfsm {
			if spec, exists := cfg.VirtioFS[k]; !exists {
				return nil, fmt.Errorf("config is missing virtiofs entry for %v", k)
			} else {
				spec.Root = vfsm[k]
				cfg.VirtioFS[k] = spec
			}
		}
		for k := range cfg.VirtioFS {
			if _, exists := vfsm[k]; !exists {
				return nil, fmt.Errorf("missing filesystem ref for %v", k)
			}
		}
	}
	return &MicroVMTask{
		Cores:  cfg.Cores,
		Memory: cfg.Memory,

		Kernel:     *kRef,
		KernelArgs: cfg.KernelArgs,
		Initrd:     initrd,

		SerialPorts: cfg.SerialPorts,
		VirtioFS:    cfg.VirtioFS,

		Input:  cfg.Input,
		Output: cfg.Output,
	}, nil
}
