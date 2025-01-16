package wantjob

import (
	"fmt"

	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/stores"
)

// OpName refers to a Operation in Want.
type OpName string

type TaskID = cadata.ID

// Task is a well defined unit of work.
type Task struct {
	Op    OpName
	Input []byte
}

func (t Task) ID() cadata.ID {
	return productHash(stores.Hash, []byte(t.Op), t.Input)
}

func (t Task) String() string {
	return fmt.Sprintf("(%s %s)", t.Op, t.Input)
}

// Executors execute Tasks
type Executor interface {
	// Execute blocks while the task is executing, and returns the result or an error.
	Execute(jc *Ctx, src cadata.Getter, task Task) ([]byte, error)
	// GetStore returns the internal store where additional data referenced by a task output may have been written.
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
