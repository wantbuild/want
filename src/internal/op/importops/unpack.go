package importops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"blobcache.io/glfs"
	"go.brendoncarroll.net/state/cadata"
)

const (
	TransformUngzip = "ungzip"
	TransformUnxz   = "unxz"
	TransformUnzstd = "unzstd"

	TransformUntar = "untar"
	TransformUnzip = "unzip"
)

type UnpackTask struct {
	X          glfs.Ref
	Transforms []string
}

type unpackBlobConfig struct {
	Transforms []string `json:"transforms"`
}

func PostUnpackTask(ctx context.Context, s cadata.PostExister, spec UnpackTask) (*glfs.Ref, error) {
	configData, err := json.Marshal(unpackBlobConfig{
		Transforms: spec.Transforms,
	})
	if err != nil {
		return nil, err
	}
	configRef, err := glfs.PostBlob(ctx, s, bytes.NewReader(configData))
	if err != nil {
		return nil, err
	}
	return glfs.PostTreeSlice(ctx, s, []glfs.TreeEntry{
		{Name: "x", FileMode: 0o777, Ref: spec.X},
		{Name: "config.json", FileMode: 0o644, Ref: *configRef},
	})
}

func GetUnpackTask(ctx context.Context, s cadata.Getter, ref glfs.Ref) (*UnpackTask, error) {
	input, err := glfs.GetAtPath(ctx, s, ref, "x")
	if err != nil {
		return nil, err
	}
	configRef, err := glfs.GetAtPath(ctx, s, ref, "config.json")
	if err != nil {
		return nil, err
	}
	config, err := loadJSON[unpackBlobConfig](ctx, s, *configRef)
	if err != nil {
		return nil, err
	}
	return &UnpackTask{X: *input, Transforms: config.Transforms}, nil
}

func (e *Executor) Unpack(ctx context.Context, dst cadata.PostExister, s cadata.Getter, spec UnpackTask) (*glfs.Ref, error) {
	op := glfs.NewAgent()
	r, err := op.GetBlob(ctx, s, spec.X)
	if err != nil {
		return nil, fmt.Errorf("unpack: while getting blob: %w", err)
	}
	return importStream(ctx, dst, r, nil, spec.Transforms)
}
