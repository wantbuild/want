package wantjob

import (
	"fmt"
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

type ErrJobNotFound struct {
	ID JobID
}

func (e ErrJobNotFound) Error() string {
	return fmt.Sprintf("job not found: %v", e.ID)
}
