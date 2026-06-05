package wantwasm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"blobcache.io/blobcache/src/bcsdk"
	"blobcache.io/glfs"
)

const (
	MaxProgramSize = 1e8
	MaxConfigSize  = 1e6
)

// WASIp1Task
type WASIp1Task struct {
	Memory  uint64
	Program []byte
	Input   glfs.Ref
	Args    []string
	Env     map[string]string
}

type wasip1Config struct {
	Args   []string          `json:"args"`
	Env    map[string]string `json:"env"`
	Memory uint64            `json:"memory"`
}

// PostTask converts a Task to a glfs Tree stored in s.
func PostWASIp1Task(ctx context.Context, mach *glfs.Machine, s bcsdk.WO, task WASIp1Task) (*glfs.Ref, error) {
	fRef, err := mach.PostBlob(ctx, s, bytes.NewReader(task.Program))
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(wasip1Config{
		Args:   task.Args,
		Env:    task.Env,
		Memory: task.Memory,
	})
	if err != nil {
		return nil, err
	}
	configRef, err := mach.PostBlob(ctx, s, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return mach.PostTreeMap(ctx, s, map[string]glfs.Ref{
		"program":     *fRef,
		"input":       task.Input,
		"config.json": *configRef,
	})
}

// GetTask parses a task from a glfs Tree.
func GetWASIp1Task(ctx context.Context, mach *glfs.Machine, s bcsdk.RO, ref glfs.Ref) (*WASIp1Task, error) {
	fRef, err := mach.GetAtPath(ctx, s, ref, "program")
	if err != nil {
		return nil, err
	}
	progData, err := mach.GetBlobBytes(ctx, s, *fRef, MaxProgramSize)
	if err != nil {
		return nil, fmt.Errorf("getting wasm function: %w", err)
	}
	inputRef, err := mach.GetAtPath(ctx, s, ref, "input")
	if err != nil {
		return nil, err
	}
	configRef, err := mach.GetAtPath(ctx, s, ref, "config.json")
	if err != nil {
		return nil, err
	}
	data, err := mach.GetBlobBytes(ctx, s, *configRef, MaxConfigSize)
	if err != nil {
		return nil, err
	}
	var config wasip1Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &WASIp1Task{
		Program: progData,
		Memory:  config.Memory,
		Input:   *inputRef,
		Args:    config.Args,
		Env:     config.Env,
	}, nil
}

// NativeGLFSTask is a Git-Like Filesystem Task
type NativeGLFSTask struct {
	Program []byte
	Input   glfs.Ref

	Memory uint64
	Args   []string
	Env    map[string]string
}

func PostNativeGLFSTask(ctx context.Context, s bcsdk.WO, x NativeGLFSTask) (*glfs.Ref, error) {
	progRef, err := glfs.PostBlob(ctx, s, bytes.NewReader(x.Program))
	if err != nil {
		return nil, err
	}
	cfgJson, err := json.Marshal(wasip1Config{})
	if err != nil {
		return nil, err
	}
	cfgRef, err := glfs.PostBlob(ctx, s, bytes.NewReader(cfgJson))
	if err != nil {
		return nil, err
	}
	return glfs.PostTreeMap(ctx, s, map[string]glfs.Ref{
		"program":     *progRef,
		"config.json": *cfgRef,
	})
}

func GetNativeTask(ctx context.Context, store bcsdk.RO, ref glfs.Ref) (*NativeGLFSTask, error) {
	mach := glfs.NewMachine()
	// program
	progRef, err := mach.GetAtPath(ctx, store, ref, "program")
	if err != nil {
		return nil, err
	}
	progData, err := mach.GetBlobBytes(ctx, store, *progRef, MaxProgramSize)
	if err != nil {
		return nil, fmt.Errorf("getting wasm function: %w", err)
	}
	// input
	inputRef, err := mach.GetAtPath(ctx, store, ref, "input")
	if err != nil {
		return nil, err
	}

	configRef, err := mach.GetAtPath(ctx, store, ref, "config.json")
	if err != nil {
		return nil, err
	}
	data, err := mach.GetBlobBytes(ctx, store, *configRef, MaxConfigSize)
	if err != nil {
		return nil, err
	}
	var config wasip1Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &NativeGLFSTask{
		Program: progData,
		Memory:  config.Memory,
		Input:   *inputRef,
		Args:    config.Args,
		Env:     config.Env,
	}, nil
}
