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

// Schema describes how objects in the store are related to inputs and outputs of tasks
type Schema string

const (
	// Schema_None reveals no information.  The data structure in the store is inscrutible.
	Schema_None = ""
	// Schema_NO_Refs means that the root does not contain any references
	// and the store should therefor be empty
	Schema_NoRefs = "norefs"
	// Schema_GLFS means the store contains a GLFS fileystem, and the root is a glfs.Ref
	Schema_GLFS = "glfs"
)

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
	Execute(jc Ctx, src cadata.Getter, task Task) Result
}

type OpFunc = func(jc Ctx, src cadata.Getter, data []byte) Result

type BasicExecutor map[OpName]OpFunc

func (exec BasicExecutor) Execute(jc Ctx, src cadata.Getter, task Task) Result {
	fn, exists := exec[task.Op]
	if !exists {
		return *Result_ErrExec(NewErrUnknownOperator(task.Op))
	}
	return fn(jc, src, task.Input)
}

type MultiExecutor map[OpName]Executor

func (me MultiExecutor) Execute(jc Ctx, src cadata.Getter, task Task) Result {
	parts := strings.SplitN(string(task.Op), ".", 2)
	e2, exists := me[OpName(parts[0])]
	if !exists {
		return *Result_ErrExec(ErrOpNotFound{Op: task.Op})
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
