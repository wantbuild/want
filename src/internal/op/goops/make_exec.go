package goops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/glfsport"
	"wantbuild.io/want/src/wantjob"
)

type MakeExecTask struct {
	Module   glfs.Ref
	ModCache *glfs.Ref
	MakeExecConfig
}

func (t MakeExecTask) Validate() error {
	if t.GOARCH == "" {
		return errors.New("GOARCH must be set")
	}
	if t.GOOS == "" {
		return errors.New("GOOS must be set")
	}
	return nil
}

type MakeExecConfig struct {
	GOARCH string `json:"GOARCH"`
	GOOS   string `json:"GOOS"`
	Main   string `json:"main"`
}

func GetMakeExecTask(ctx context.Context, s cadata.Getter, x glfs.Ref) (*MakeExecTask, error) {
	moduleRef, err := glfs.GetAtPath(ctx, s, x, "module")
	if err != nil {
		return nil, err
	}
	modcacheRef, err := glfs.GetAtPath(ctx, s, x, "modcache")
	if err != nil && !glfs.IsErrNoEnt(err) {
		return nil, err
	}
	configRef, err := glfs.GetAtPath(ctx, s, x, "config.json")
	if err != nil {
		return nil, err
	}
	configData, err := glfs.GetBlobBytes(ctx, s, *configRef, 1e9)
	if err != nil {
		return nil, err
	}
	var config MakeExecConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, err
	}
	return &MakeExecTask{
		ModCache:       modcacheRef,
		Module:         *moduleRef,
		MakeExecConfig: config,
	}, nil
}

func PostMakeExecTask(ctx context.Context, s cadata.Poster, x MakeExecTask) (*glfs.Ref, error) {
	data, err := json.Marshal(x.MakeExecConfig)
	if err != nil {
		return nil, err
	}
	configRef, err := glfs.PostBlob(ctx, s, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	ents := []glfs.TreeEntry{
		{Name: "module", FileMode: 0o777, Ref: x.Module},
		{Name: "config.json", FileMode: 0o777, Ref: *configRef},
	}
	if x.ModCache != nil {
		ents = append(ents, glfs.TreeEntry{Name: "modcache", FileMode: 0o777, Ref: *x.ModCache})
	}
	return glfs.PostTreeEntries(ctx, s, ents)
}

func (e *Executor) MakeExec(jc wantjob.Ctx, src cadata.Getter, task MakeExecTask) (*glfs.Ref, error) {
	ctx := jc.Context
	if err := task.Validate(); err != nil {
		return nil, err
	}
	dir, cleanup, err := e.mkdirTemp(ctx, "makeExec-")
	if err != nil {
		return nil, err
	}
	defer cleanup()

	inPath := filepath.Join(dir, "in")
	outPath := filepath.Join(dir, "out")
	var entryPath string
	if task.Main != "" {
		if !filepath.IsLocal(task.Main) {
			return nil, fmt.Errorf("main path is not local %s", task.Main)
		}
		entryPath = "./" + task.Main
	}

	// setup module
	exp := glfsport.Exporter{
		Dir:   dir,
		Cache: glfsport.NullCache{},
		Store: src,
	}
	if err := exp.Export(ctx, task.Module, "in"); err != nil {
		return nil, err
	}
	if task.ModCache != nil {
		if err := exp.Export(ctx, *task.ModCache, "modcache"); err != nil {
			return nil, err
		}
	}

	args := []string{"build",
		"-v",
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
	jc.Infof("GOARCH=%s GOOS=%s", task.GOARCH, task.GOOS)

	df := jc.InfoSpan("go build")
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	df()

	imp := glfsport.Importer{
		Store: jc.Dst,
		Dir:   dir,
		Cache: glfsport.NullCache{},
	}
	return imp.Import(ctx, "out")
}
