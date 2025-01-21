package qemuops

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/op/glfsops"
	"wantbuild.io/want/lib/wantjob"
)

const (
	OpAmd64MicroVMVirtioFS = wantjob.OpName("amd64_microvm_virtiofs")
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
	// Assert is used to make assertions about the output and fail the Task if they are not met.
	Assert *glfsops.Assertions
}

// MicroVMConfig is the config file for a MicroVMTask
type MicroVMConfig struct {
	Cores  uint32   `json:"cores"`
	Memory uint64   `json:"memory"`
	Init   string   `json:"init,omitempty"`
	Args   []string `json:"args"`
	Output string   `json:"output,omitempty"`
}

func PostMicroVMTask(ctx context.Context, s cadata.Store, x MicroVMTask) (*glfs.Ref, error) {
	ag := glfs.NewAgent()
	configData, err := json.Marshal(MicroVMConfig{
		Cores:  x.Cores,
		Memory: x.Memory,
		Args:   x.Args,
		Output: x.Output,
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
	if x.Assert != nil {
		aRef, err := glfsops.PostAssertions(ctx, s, *x.Assert)
		if err != nil {
			return nil, err
		}
		ents = append(ents, glfs.TreeEntry{
			Name:     "assert",
			FileMode: 0o777,
			Ref:      *aRef,
		})
	}
	return ag.PostTreeEntries(ctx, s, ents)
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
	var assertions *glfsops.Assertions
	assertRef, err := ag.GetAtPath(ctx, s, x, "assert")
	if err != nil && !glfs.IsErrNoEnt(err) {
		return nil, err
	} else if err == nil {
		assertions, err = glfsops.GetAssertions(ctx, s, *assertRef)
		if err != nil {
			return nil, err
		}
	}
	return &MicroVMTask{
		Cores:  cf.Cores,
		Memory: cf.Memory,
		Init:   cf.Init,
		Args:   cf.Args,
		Output: cf.Output,

		Kernel: *kRef,
		Root:   *rRef,

		Assert: assertions,
	}, nil
}
