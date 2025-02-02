package glfstasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/internal/wantdb"
	"wantbuild.io/want/src/wantjob"
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
	if out != nil {
		return MarshalGLFSRef(*out), err
	} else {
		return nil, err
	}
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

func FastSync(ctx context.Context, dst cadata.PostExister, src cadata.Getter, root glfs.Ref) error {
	rootData := MarshalGLFSRef(root)
	var err error
	switch dst := dst.(type) {
	case *wantdb.DBStore:
		err = dst.Pull(ctx, rootData)
	case *wantdb.TxStore:
		err = dst.Pull(ctx, rootData)
	default:
		return glfs.Sync(ctx, dst, src, root)
	}
	if errors.Is(err, wantdb.ErrPullNoMatch) {
		return glfs.Sync(ctx, dst, src, root)
	}
	return err
}

func Check(ctx context.Context, src cadata.Getter, root glfs.Ref) error {
	return check(ctx, src, root, nil)
}

func check(ctx context.Context, src cadata.Getter, root glfs.Ref, history []string) error {
	if yes, err := stores.ExistsOnGet(ctx, src, root.CID); err != nil {
		return err
	} else if !yes {
		return fmt.Errorf("integrity check failed, store %v: %T is missing %v", src, src, root.CID)
	}
	if root.Type == glfs.TypeTree {
		tree, err := glfs.GetTreeSlice(ctx, src, root, 1e6)
		if err != nil {
			return err
		}
		for _, ent := range tree {
			history = append(history, ent.Name)
			if err := check(ctx, src, ent.Ref, history); err != nil {
				return fmt.Errorf("check entry at %v: %w", history, err)
			}
		}
	}
	return nil
}
