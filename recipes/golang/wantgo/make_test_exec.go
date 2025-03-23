// DO NOT EDIT
// This has been duplicated from src/internal/op/goops
// 
// cp src/internal/op/goops/make_test_exec.go recipes/golang/wantgo
//
package wantgo

import (
	"bytes"
	"context"
	"encoding/json"

	"blobcache.io/glfs"
	"go.brendoncarroll.net/state/cadata"
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
