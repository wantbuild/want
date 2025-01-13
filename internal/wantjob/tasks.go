package wantjob

import (
	"context"
	"fmt"
	"slices"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/exp/slices2"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/stores"
)

// OpName refers to a Operation in Want.
type OpName string

// OpSet is a set of OpNames
type OpSet []OpName

func NewOpSet(ops ...OpName) OpSet {
	ops = slices.Clone(ops)
	slices.Sort(ops)
	ops = slices2.DedupSorted(ops)
	return OpSet(ops)
}

func (s OpSet) Add(x OpName) OpSet {
	return NewOpSet(append(s, x)...)
}

// Task is a well defined unit of work.
type Task struct {
	Op    OpName
	Input glfs.Ref
}

func (t Task) ID() cadata.ID {
	data, err := t.Input.MarshalBinary()
	if err != nil {
		panic(err)
	}
	return productHash(stores.Hash, []byte(t.Op), data)
}

func (t Task) String() string {
	return fmt.Sprintf("%s(%v)", t.Op, t.Input.CID)
}

// Executors execute Tasks
type Executor interface {
	Compute(ctx context.Context, jc *JobCtx, src cadata.Getter, input Task) (*glfs.Ref, error)
	GetStore() cadata.Getter
}

func productHash(hf cadata.HashFunc, xs ...[]byte) cadata.ID {
	var data []byte
	for _, x := range xs {
		h := hf(x)
		data = append(data, h[:]...)
	}
	return hf(data)
}
