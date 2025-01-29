package stringsets

import (
	"fmt"
	"regexp"

	"go.brendoncarroll.net/exp/maybe"
)

type Not struct {
	X Set
}

func (n Not) Contains(x string) bool {
	return !n.X.Contains(x)
}

func (n Not) Regexp() *regexp.Regexp {
	x := n.X.Regexp()
	return regexp.MustCompile(fmt.Sprintf("^(?!%s).*$", x.String()))
}

func (n Not) superset(sub Set) maybe.Maybe[bool] {
	if isTrue(intersects(n.X, sub)) {
		return maybe.Just(false)
	}
	if isFalse(intersects(n.X, sub)) {
		return maybe.Just(true)
	}
	if isTrue(n.X.superset(sub)) {
		return maybe.Just(false)
	}
	return maybe.Nothing[bool]()
}

func (n Not) intersects(o Set) maybe.Maybe[bool] {
	if isTrue(n.X.superset(o)) {
		return maybe.Just(false)
	}
	if isFalse(intersects(n.X, o)) {
		return maybe.Just(true)
	}
	return maybe.Nothing[bool]()
}

func (n Not) simplify() Set {
	switch x := n.X.(type) {
	case Not:
		return x.X.simplify()
	default:
		return Not{x.simplify()}
	}
}

func (n Not) isDNF() bool {
	switch n.X.(type) {
	case And, Or, Not:
		return false
	default:
		return true
	}
}

func (n Not) String() string {
	return fmt.Sprintf("!%v", n.X)
}
