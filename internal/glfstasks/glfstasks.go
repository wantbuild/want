package glfstasks

import (
	"context"
	"encoding/json"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/lib/wantjob"
)

func Do(ctx context.Context, sys wantjob.System, src cadata.Getter, op wantjob.OpName, x glfs.Ref) (*glfs.Ref, cadata.Getter, error) {
	out, dst, err := wantjob.Do(ctx, sys, src, wantjob.Task{
		Op:    op,
		Input: MarshalGLFSRef(x),
	})
	if err != nil {
		return nil, nil, err
	}
	if err := out.Err(); err != nil {
		return nil, nil, err
	}
	ref, err := ParseGLFSRef(out.Data)
	if err != nil {
		return nil, nil, err
	}
	return ref, dst, nil
}

func Exec(x []byte, fn func(x glfs.Ref) (*glfs.Ref, error)) ([]byte, error) {
	in, err := ParseGLFSRef(x)
	if err != nil {
		return nil, err
	}
	out, err := fn(*in)
	if err != nil {
		return nil, err
	}
	return MarshalGLFSRef(*out), nil
}

func ParseGLFSRef(x []byte) (*glfs.Ref, error) {
	var ret glfs.Ref
	if err := json.Unmarshal(x, &ret); err != nil {
		return nil, err
	}
	return &ret, nil
}

func MarshalGLFSRef(x glfs.Ref) []byte {
	data, err := json.Marshal(x)
	if err != nil {
		panic(err)
	}
	return data
}

func Success(x glfs.Ref) *wantjob.Result {
	return &wantjob.Result{Data: MarshalGLFSRef(x)}
}
