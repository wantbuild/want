package goops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"blobcache.io/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/glfsport"
	"wantbuild.io/want/src/wantjob"
)

const MakeTestExecConfigFilename = "makeTestExec.json"

type MakeTestExecTask struct {
	Module   glfs.Ref
	ModCache *glfs.Ref

	MakeTestExecConfig
}

type MakeTestExecConfig struct {
	GOARCH string
	GOOS   string
	Path   string
}

func GetMakeTestExecTask(ctx context.Context, s cadata.Getter, x glfs.Ref) (*MakeTestExecTask, error) {
	moduleRef, err := glfs.GetAtPath(ctx, s, x, "module")
	if err != nil {
		return nil, err
	}
	modcacheRef, err := glfs.GetAtPath(ctx, s, x, "modcache")
	if err != nil && !glfs.IsErrNoEnt(err) {
		return nil, err
	}
	configRef, err := glfs.GetAtPath(ctx, s, x, MakeTestExecConfigFilename)
	if err != nil {
		return nil, err
	}
	configData, err := glfs.GetBlobBytes(ctx, s, *configRef, 1e6)
	if err != nil {
		return nil, err
	}
	var config MakeTestExecConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, err
	}
	return &MakeTestExecTask{
		ModCache:           modcacheRef,
		Module:             *moduleRef,
		MakeTestExecConfig: config,
	}, nil
}

func PostMakeTestExecTask(ctx context.Context, s cadata.PostExister, x MakeTestExecTask) (*glfs.Ref, error) {
	configData, err := json.Marshal(x.MakeTestExecConfig)
	if err != nil {
		return nil, err
	}
	configRef, err := glfs.PostBlob(ctx, s, bytes.NewReader(configData))
	if err != nil {
		return nil, err
	}
	m := map[string]glfs.Ref{
		"module":                   x.Module,
		MakeTestExecConfigFilename: *configRef,
	}
	if x.ModCache != nil {
		m["modcache"] = *x.ModCache
	}
	return glfs.PostTreeMap(ctx, s, m)
}

func (e *Executor) MakeTestExec(jc wantjob.Ctx, src cadata.Getter, task MakeTestExecTask) (*glfs.Ref, error) {
	ctx := jc.Context
	dir, cleanup, err := e.mkdirTemp(ctx, "makeTestExec-")
	if err != nil {
		return nil, err
	}
	defer cleanup()

	inPath := filepath.Join(dir, "in")
	outPath := filepath.Join(dir, "out")
	var entryPath string
	if task.Path != "" {
		if !filepath.IsLocal(task.Path) {
			return nil, fmt.Errorf("entry path is not local %s", task.Path)
		}
		entryPath = filepath.Join(inPath, task.Path)
	}

	// setup module
	exp := glfsport.Exporter{
		Dir:   dir,
		Cache: glfsport.NullCache{},
		Store: src,
	}
	jc.Infof("exporting source: begin")
	if err := exp.Export(ctx, task.Module, "in"); err != nil {
		return nil, err
	}
	jc.Infof("exporting source: done")
	jc.Infof("exporting modcache: begin")
	if task.ModCache != nil {
		if err := exp.Export(ctx, *task.ModCache, "modcache"); err != nil {
			return nil, err
		}
	}
	jc.Infof("exporting modcache: done")

	args := []string{"test",
		"-v",
		"-c",
		"-o", outPath,
		"-trimpath",
		"-ldflags", `-s -w -buildid=`,
		"-buildvcs=false",
	}
	if entryPath != "" {
		args = append(args, entryPath)
	}
	jc.Infof("go %v", args)
	cmd := e.newCommand(ctx, goConfig{
		GOARCH:     task.GOARCH,
		GOOS:       task.GOOS,
		GOMODCACHE: filepath.Join(dir, "modcache"),
	}, args...)
	cmd.Dir = inPath
	cmd.Stdout = jc.Writer("stdout")
	cmd.Stderr = jc.Writer("stderr")

	jc.Infof("build tests for package %s", task.Path)
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	jc.Infof("done building test executable")
	imp := glfsport.Importer{
		Store: jc.Dst,
		Dir:   dir,
		Cache: glfsport.NullCache{},
	}
	return imp.Import(ctx, "out")
}
