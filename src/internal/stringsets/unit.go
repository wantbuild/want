package stringsets

import (
	"regexp"

	"go.brendoncarroll.net/exp/maybe"
)

type Unit string

func (k Unit) Contains(x string) bool {
	return string(k) == x
}

func (k Unit) Regexp() *regexp.Regexp {
	return regexp.MustCompile("^" + regexp.QuoteMeta(string(k)) + "$")
}

func (k Unit) String() string {
	return string(k)
}

func (k Unit) intersects(x Set) maybe.Maybe[bool] {
	yes := false
	switch x := x.(type) {
	case Unit:
		yes = k == x
	default:
		return maybe.Nothing[bool]()
	}
	return maybe.Just(yes)
}

func (k Unit) superset(x Set) maybe.Maybe[bool] {
	switch x := x.(type) {
	case Empty:
		return maybe.Just(true)
	case Top:
		return maybe.Just(false)
	case Unit:
		return maybe.Just(k == x)
	default:
		return maybe.Nothing[bool]()
	}
}

func (k Unit) simplify() Set {
	return k
}

func (k Unit) isDNF() bool {
	return true
}
