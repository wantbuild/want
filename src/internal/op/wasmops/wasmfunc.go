package wasmops

import (
	"encoding/json"
	"errors"
	"fmt"

	"blobcache.io/glfs"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/wantjob"
	"wantbuild.io/want/src/wantwasm"
)

func (e *Executor) ExecNativeGLFS(jc wantjob.Ctx, s cadata.Getter, task wantwasm.NativeGLFSTask) (*glfs.Ref, error) {
	ctx := jc.Context
	if task.Memory == 0 {
		task.Memory = 1e9
	}
	r := wazero.NewRuntimeWithConfig(ctx, newRuntimeConfig(task.Memory))
	defer r.Close(ctx)
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, r); err != nil {
		return nil, err
	}

	inputData, err := json.Marshal(task.Input)
	if err != nil {
		return nil, err
	}

	dst := jc.Dst
	src := stores.Union{s, dst}
	var out *glfs.Ref
	_, err = r.NewHostModuleBuilder("want").
		NewFunctionBuilder().WithFunc(func(ptr, l uint32) int32 {
		mod := r.Module("main")
		n := min(int(l), len(inputData))
		ok := mod.Memory().Write(ptr, inputData[:n])
		if !ok {
			return -1
		}
		return int32(n)
	}).Export("input").
		NewFunctionBuilder().WithFunc(func(ptr, l uint32) int32 {
		mod := r.Module("main")
		data, ok := mod.Memory().Read(ptr, l)
		if !ok {
			return -1
		}
		var ref glfs.Ref
		if err := json.Unmarshal(data, &ref); err != nil {
			jc.Infof("error %v", err)
			return -1
		}
		out = &ref
		return 0
	}).Export("output").
		NewFunctionBuilder().WithFunc(func(idBuf, dataPtr, dataLen uint32) int32 {
		mod := r.Module("main")
		data, ok := mod.Memory().Read(dataPtr, dataLen)
		if !ok {
			return -1
		}
		id, err := dst.Post(ctx, data)
		if err != nil {
			jc.Infof("%v", err)
			return -1
		}
		if !mod.Memory().Write(idBuf, id[:]) {
			jc.Infof("post: error writing to idBuf")
			return -1
		}
		return 0
	}).Export("post").
		NewFunctionBuilder().WithFunc(func(dataBuf, dataLen, idBuf uint32) int32 {
		mod := r.Module("main")
		idData, ok := mod.Memory().Read(idBuf, 32)
		if !ok {
			return -1
		}
		id := cadata.IDFromBytes(idData)
		buf := make([]byte, src.MaxSize())
		n, err := src.Get(ctx, id, buf)
		if err != nil && !cadata.IsNotFound(err) {
			jc.Infof("%v", err)
			return -1
		} else if cadata.IsNotFound(err) {
			if !mod.Memory().Write(dataBuf, id[:int(min(dataLen, 32))]) {
				jc.Infof("get: error writing to dataBuf")
				return -1
			}
			return 32
		}
		if !mod.Memory().Write(dataBuf, buf[:min(n, int(dataLen))]) {
			jc.Infof("get: error writing to dataBuf")
			return -1
		}
		return int32(n)
	}).Export("get").
		Instantiate(ctx)
	if err != nil {
		return nil, err
	}

	cfg := wazero.NewModuleConfig().
		WithStdout(jc.Writer("stdout")).
		WithStderr(jc.Writer("stderr")).
		WithArgs(task.Args...)
	cfg = cfg.WithEnv("GOMEMLIMIT", fmt.Sprintf("%dB", task.Memory*3/4))
	for k, v := range task.Env {
		cfg = cfg.WithEnv(k, v)
	}
	cfg = cfg.WithName("main")

	jc.Infof("memory: %v", task.Memory)
	jc.Infof("begin")
	_, err = r.InstantiateWithConfig(ctx, task.Program, cfg)
	if err != nil {
		return nil, err
	}
	jc.Infof("done")
	if out == nil {
		return nil, errors.New("no output")
	}
	return out, nil
}

func newRuntimeConfig(memory uint64) wazero.RuntimeConfig {
	const pageSize = 1 << 16
	rtCfg := wazero.NewRuntimeConfig().
		WithMemoryLimitPages(uint32(memory / pageSize)).
		WithDebugInfoEnabled(true)
	return rtCfg
}
