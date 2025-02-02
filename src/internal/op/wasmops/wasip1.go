package wasmops

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/blobcache/glfs"
	"github.com/blobcache/glfs/glfsiofs"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/glfsport"
	"wantbuild.io/want/src/wantjob"
)

func ExecWASIp1(jc wantjob.Ctx, src cadata.Getter, task WASIp1Task) (*glfs.Ref, error) {
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
