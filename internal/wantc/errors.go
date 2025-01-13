package wantc

import (
	"fmt"

	"wantbuild.io/want/internal/stringsets"
)

type ErrCycle struct {
	Cycle []string
}

func (e ErrCycle) Error() string {
	return fmt.Sprintln("cycle: ", e.Cycle)
}

type ErrConflict struct {
	Overlapping []stringsets.Set
}

func (e ErrConflict) Error() string {
	return fmt.Sprintf("conflict: %v", e.Overlapping)
}
