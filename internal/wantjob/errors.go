package wantjob

import (
	"fmt"

	"github.com/blobcache/glfs"
)

type ErrOpNotFound struct {
	Op OpName
}

func NewErrUnknownOperator(op OpName) ErrOpNotFound {
	return ErrOpNotFound{Op: op}
}

func (e ErrOpNotFound) Error() string {
	return fmt.Sprintf("op not found: %v", e.Op)
}

type ErrInvalidInput struct {
	Input glfs.Ref
	Msg   string
}

func NewErrInvalidInput(input glfs.Ref, msg string) ErrInvalidInput {
	return ErrInvalidInput{Input: input, Msg: msg}
}

func (e ErrInvalidInput) Error() string {
	return fmt.Sprintf("invalid input. input=%v msg=%v", e.Input, e.Msg)
}

type ErrJobNotFound struct {
	ID JobID
}

func (e ErrJobNotFound) Error() string {
	return fmt.Sprintf("job not found: %v", e.ID)
}
