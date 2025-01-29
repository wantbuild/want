package stringsets

import (
	"regexp"

	"go.brendoncarroll.net/exp/maybe"
)

type Single string

func (k Single) Contains(x string) bool {
	return string(k) == x
}

func (k Single) Regexp() *regexp.Regexp {
	return regexp.MustCompile("^" + regexp.QuoteMeta(string(k)) + "$")
}

func (k Single) String() string {
	return string(k)
}

func (k Single) intersects(x Set) maybe.Maybe[bool] {
	yes := false
	switch x := x.(type) {
	case Single:
		yes = k == x
	default:
		return maybe.Nothing[bool]()
	}
	return maybe.Just(yes)
}

func (k Single) superset(x Set) maybe.Maybe[bool] {
	switch x := x.(type) {
	case Empty:
		return maybe.Just(true)
	case Top:
		return maybe.Just(false)
	case Single:
		return maybe.Just(k == x)
	default:
		return maybe.Nothing[bool]()
	}
}

func (k Single) simplify() Set {
	return k
}

func (k Single) isDNF() bool {
	return true
}
