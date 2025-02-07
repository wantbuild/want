package wantc

import (
	"fmt"

	"wantbuild.io/want/src/internal/stringsets"
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

type ErrMissingDep struct {
	Name string
}

func (e ErrMissingDep) Error() string {
	return fmt.Sprintf("module is missing dependency for %v", e.Name)
}

type ErrExtraDep struct {
	Name string
}

func (e ErrExtraDep) Error() string {
	return fmt.Sprintf("extra dependency %v is not needed", e.Name)
}
