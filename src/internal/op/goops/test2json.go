package goops

import (
	"bytes"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/wantjob"
)

func (e *Executor) Test2JSON(jc wantjob.Ctx, src cadata.Getter, ref glfs.Ref) (*glfs.Ref, error) {
	ctx := jc.Context
	r, err := glfs.GetBlob(ctx, src, ref)
	if err != nil {
		return nil, err
	}
	args := []string{
		"tool", "test2json",
	}
	cmd := e.newCommand(ctx, goConfig{}, args...)
	cmd.Stdin = r
	data, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	return glfs.PostBlob(ctx, jc.Dst, bytes.NewReader(data))
}
