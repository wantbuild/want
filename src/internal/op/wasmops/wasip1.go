package wasmops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/blobcache/glfs"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/glfsiofs"
	"wantbuild.io/want/src/internal/glfsport"
	"wantbuild.io/want/src/wantjob"
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

type configFile struct {
	Args   []string          `json:"args"`
	Env    map[string]string `json:"env"`
	Memory uint64            `json:"memory"`
}

// PostTask converts a Task to a glfs Tree stored in s.
func PostWASIp1Task(ctx context.Context, ag *glfs.Agent, s cadata.Poster, task WASIp1Task) (*glfs.Ref, error) {
	fRef, err := ag.PostBlob(ctx, s, bytes.NewReader(task.Program))
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(configFile{
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
	var config configFile
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

func ComputeWASIp1(jc wantjob.Ctx, src cadata.Getter, task WASIp1Task) (*glfs.Ref, error) {
	ctx := jc.Context
	if task.Memory == 0 {
		task.Memory = 4 * 1e9
	}
	r := wazero.NewRuntimeWithConfig(ctx, newRuntimeConfig(task.Memory))
	defer r.Close(ctx)
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, r); err != nil {
		return nil, err
	}

	scratchDir, err := os.MkdirTemp("", "wasmops-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(scratchDir)
	outDir := filepath.Join(scratchDir, "output")
	if err := os.Mkdir(outDir, 0o755); err != nil {
		return nil, err
	}
	defer os.RemoveAll(outDir)
	tmpDir := filepath.Join(scratchDir, "tmp")
	if err := os.Mkdir(tmpDir, 0o755); err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	fsCfg := wazero.NewFSConfig().
		WithFSMount(glfsiofs.New(src, task.Input), "/input").
		WithDirMount(outDir, "/output").
		WithDirMount(tmpDir, "/tmp")

	cfg := wazero.NewModuleConfig().
		WithStdout(jc.Writer("stdout")).
		WithStderr(jc.Writer("stderr")).
		WithFSConfig(fsCfg).
		WithArgs(task.Args...)
	jc.Infof("memory: %v", task.Memory)
	cfg = cfg.WithEnv("GOMEMLIMIT", fmt.Sprintf("%dB", task.Memory*3/4))
	cfg = cfg.WithEnv("GOGC", "10")
	for k, v := range task.Env {
		cfg = cfg.WithEnv(k, v)
	}

	_, err = r.InstantiateWithConfig(ctx, task.Program, cfg)
	if err != nil {
		return nil, err
	}
	// Collect output
	imp := glfsport.Importer{
		Cache: glfsport.NullCache{},
		Store: jc.Dst,
		Dir:   outDir,
	}
	return imp.Import(ctx, "")
}
