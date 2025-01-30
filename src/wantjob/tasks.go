package wantjob

import (
	"fmt"
	"strings"

	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/stores"
)

// MaxInputSize is the maximum size of a Task.Input
const MaxInputSize = 1 << 16

// OpName refers to a Operation in Want.
type OpName string

// TaskID uniquely identifies Tasks
type TaskID = cadata.ID

// Task is a well defined unit of work.
type Task struct {
	Op    OpName
	Input []byte
}

func (t Task) ID() TaskID {
	return productHash(stores.Hash, []byte(t.Op), t.Input)
}

func (t Task) String() string {
	return fmt.Sprintf("(%s %s)", t.Op, t.Input)
}

// Executors execute Tasks
type Executor interface {
	// Execute blocks while the task is executing, and returns the result or an error.
	Execute(jc Ctx, src cadata.Getter, task Task) ([]byte, error)
}

type OpFunc = func(jc Ctx, src cadata.Getter, data []byte) ([]byte, error)

type BasicExecutor map[OpName]OpFunc

func (exec BasicExecutor) Execute(jc Ctx, src cadata.Getter, task Task) ([]byte, error) {
	fn, exists := exec[task.Op]
	if !exists {
		return nil, NewErrUnknownOperator(task.Op)
	}
	return fn(jc, src, task.Input)
}

type MultiExecutor map[OpName]Executor

func (me MultiExecutor) Execute(jc Ctx, src cadata.Getter, task Task) ([]byte, error) {
	parts := strings.SplitN(string(task.Op), ".", 2)
	e2, exists := me[OpName(parts[0])]
	if !exists {
		return nil, ErrOpNotFound{Op: task.Op}
	}
	return e2.Execute(jc, src, Task{
		Op:    OpName(parts[1]),
		Input: task.Input,
	})
}

func productHash(hf cadata.HashFunc, xs ...[]byte) cadata.ID {
	var data []byte
	for _, x := range xs {
		h := hf(x)
		data = append(data, h[:]...)
	}
	return hf(data)
}
