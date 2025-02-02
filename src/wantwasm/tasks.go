package wantwasm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"
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
func PostWASIp1Task(ctx context.Context, ag *glfs.Agent, s cadata.PostExister, task WASIp1Task) (*glfs.Ref, error) {
	fRef, err := ag.PostBlob(ctx, s, bytes.NewReader(task.Program))
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
	configRef, err := ag.PostBlob(ctx, s, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return ag.PostTreeMap(ctx, s, map[string]glfs.Ref{
		"program":     *fRef,
		"input":       task.Input,
		"config.json": *configRef,
	})
}

// GetTask parses a task from a glfs Tree.
func GetWASIp1Task(ctx context.Context, ag *glfs.Agent, s cadata.Getter, ref glfs.Ref) (*WASIp1Task, error) {
	fRef, err := ag.GetAtPath(ctx, s, ref, "program")
	if err != nil {
		return nil, err
	}
	progData, err := ag.GetBlobBytes(ctx, s, *fRef, MaxProgramSize)
	if err != nil {
		return nil, fmt.Errorf("getting wasm function: %w", err)
	}
	inputRef, err := ag.GetAtPath(ctx, s, ref, "input")
	if err != nil {
		return nil, err
	}
	configRef, err := ag.GetAtPath(ctx, s, ref, "config.json")
	if err != nil {
		return nil, err
	}
	data, err := ag.GetBlobBytes(ctx, s, *configRef, MaxConfigSize)
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

func PostNativeGLFSTask(ctx context.Context, s cadata.PostExister, x NativeGLFSTask) (*glfs.Ref, error) {
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

func GetNativeTask(ctx context.Context, store cadata.Getter, ref glfs.Ref) (*NativeGLFSTask, error) {
	ag := glfs.NewAgent()
	// program
	progRef, err := ag.GetAtPath(ctx, store, ref, "program")
	if err != nil {
		return nil, err
	}
	progData, err := ag.GetBlobBytes(ctx, store, *progRef, MaxProgramSize)
	if err != nil {
		return nil, fmt.Errorf("getting wasm function: %w", err)
	}
	// input
	inputRef, err := ag.GetAtPath(ctx, store, ref, "input")
	if err != nil {
		return nil, err
	}

	configRef, err := ag.GetAtPath(ctx, store, ref, "config.json")
	if err != nil {
		return nil, err
	}
	data, err := ag.GetBlobBytes(ctx, store, *configRef, MaxConfigSize)
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
