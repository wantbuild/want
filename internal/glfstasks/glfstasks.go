package glfstasks

import (
	"encoding/json"

	"github.com/blobcache/glfs"

	"wantbuild.io/want/internal/wantjob"
)

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
