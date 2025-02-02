package wantqemu

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"
)

// MicroVMTask is an Amd64 Linux MicroVM Task
type MicroVMTask struct {
	Cores uint32
	// Memory is the memory in bytes
	Memory uint64

	// Kernel is the linux kernel image used to boot the VM
	Kernel glfs.Ref
	// Root is the root filesystem to boot
	Root glfs.Ref
	// Init is the path to init
	Init string
	// Args are the arguments passed to init
	Args []string

	// Output is the subpath within the root to use as the result of the Task.
	Output string
}

// MicroVMConfig is the config file for a MicroVMTask
type MicroVMConfig struct {
	Cores  uint32   `json:"cores"`
	Memory uint64   `json:"memory"`
	Init   string   `json:"init,omitempty"`
	Args   []string `json:"args"`
	Output string   `json:"output,omitempty"`
}

func PostMicroVMTask(ctx context.Context, s cadata.PostExister, x MicroVMTask) (*glfs.Ref, error) {
	ag := glfs.NewAgent()
	configData, err := json.Marshal(MicroVMConfig{
		Cores:  x.Cores,
		Memory: x.Memory,
		Args:   x.Args,
		Init:   x.Init,
		Output: strings.Trim(x.Output, "/"),
	})
	if err != nil {
		return nil, err
	}
	cRef, err := ag.PostBlob(ctx, s, bytes.NewReader(configData))
	if err != nil {
		return nil, err
	}
	ents := []glfs.TreeEntry{
		{Name: "config.json", FileMode: 0o644, Ref: *cRef},
		{Name: "kernel", FileMode: 0o644, Ref: x.Kernel},
		{Name: "root", FileMode: 0o644, Ref: x.Root},
	}
	return ag.PostTreeSlice(ctx, s, ents)
}

func GetMicroVMTask(ctx context.Context, s cadata.Getter, x glfs.Ref) (*MicroVMTask, error) {
	ag := glfs.NewAgent()
	configRef, err := ag.GetAtPath(ctx, s, x, "config.json")
	if err != nil {
		return nil, err
	}
	configData, err := ag.GetBlobBytes(ctx, s, *configRef, 1e6)
	if err != nil {
		return nil, err
	}
	var cf MicroVMConfig
	if err := json.Unmarshal(configData, &cf); err != nil {
		return nil, err
	}
	kRef, err := ag.GetAtPath(ctx, s, x, "kernel")
	if err != nil {
		return nil, err
	}
	rRef, err := ag.GetAtPath(ctx, s, x, "root")
	if err != nil {
		return nil, err
	}
	return &MicroVMTask{
		Cores:  cf.Cores,
		Memory: cf.Memory,
		Init:   cf.Init,
		Args:   cf.Args,
		Output: cf.Output,

		Kernel: *kRef,
		Root:   *rRef,
	}, nil
}
