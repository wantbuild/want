package stringsets

import (
	"fmt"
	"reflect"

	"go.brendoncarroll.net/exp/maybe"
)

type And struct {
	L, R Set
}

func (a And) Contains(x string) bool {
	return a.L.Contains(x) && a.R.Contains(x)
}

func (a And) intersects(x Set) maybe.Maybe[bool] {
	li := intersects(a.L, x)
	ri := intersects(a.R, x)

	if isFalse(li) || isFalse(ri) {
		return maybe.Just(false)
	}
	return maybe.Nothing[bool]()
}

func (a And) superset(x Set) maybe.Maybe[bool] {
	ls := superset(a.L, x)
	rs := superset(a.R, x)
	if isFalse(ls) || isFalse(rs) {
		return maybe.Just(false)
	}
	if isTrue(ls) && isTrue(rs) {
		return maybe.Just(true)
	}
	return maybe.Nothing[bool]()
}

func (a And) String() string {
	return fmt.Sprintf("(%v && %v)", a.L, a.R)
}

func (a And) simplify() (ret Set) {
	a = a.dist()
	l, r := a.L.simplify(), a.R.simplify()
	switch {
	case reflect.DeepEqual(l, r):
		return l
	case isTrue(superset(r, l)):
		return l
	case isTrue(superset(l, r)):
		return r
	case isTrue(superset(Not{r}, l)) || isTrue(superset(Not{l}, r)):
		return Empty{}
	case isFalse(intersects(a.L, a.R)):
		return Empty{}
	default:
		return And{L: l, R: r}
	}
}

func (a And) isDNF() bool {
	isDNF := func(x Set) bool {
		switch x := x.(type) {
		case Or:
			return false
		default:
			return x.isDNF()
		}
	}
	return isDNF(a.L) && isDNF(a.R)
}

func (a And) dist() And {
	l, r := a.L, a.R

	la, lok := l.(And)
	ra, rok := r.(And)
	switch {
	// (la.l & la.r) & (a.r)
	case lok && !rok:
		return And{
			L: And{L: la.L, R: r},
			R: And{L: la.R, R: r},
		}.dist()

	// a.l & (ra.l & ra.r )
	case rok && !lok:
		return And{
			L: And{L: l, R: ra.L},
			R: And{L: l, R: ra.R},
		}.dist()

	default:
		return a
	}
}

func (a And) deMorgan() Set {
	return Not{Or{
		L: Not{a.L},
		R: Not{a.R},
	}}
}

type Or struct {
	L, R Set
}

func (o Or) Contains(x string) bool {
	return o.L.Contains(x) || o.R.Contains(x)
}

func (o Or) intersects(x Set) maybe.Maybe[bool] {
	li := intersects(o.L, x)
	ri := intersects(o.R, x)
	if isTrue(li) || isTrue(ri) {
		return maybe.Just(true)
	}
	if isFalse(li) && isFalse(ri) {
		return maybe.Just(false)
	}
	return maybe.Nothing[bool]()
}

func (o Or) superset(x Set) maybe.Maybe[bool] {
	ls := superset(o.L, x)
	rs := superset(o.R, x)
	if isTrue(ls) || isTrue(rs) {
		return maybe.Just(true)
	}
	if isFalse(ls) && isFalse(rs) {
		return maybe.Just(false)
	}
	return maybe.Nothing[bool]()
}

func (o Or) String() string {
	return fmt.Sprintf("(%v || %v)", o.L, o.R)
}

func (o Or) simplify() Set {
	// o = o.dist()
	l, r := o.L.simplify(), o.R.simplify()
	switch {
	case reflect.DeepEqual(l, Top{}) || reflect.DeepEqual(r, Top{}):
		return Top{}
	case isTrue(superset(l, r)):
		return l
	case isTrue(superset(r, l)):
		return r
	case isTrue(superset(l, Not{r})) || isTrue(superset(r, Not{l})):
		return Top{}
	default:
		return Or{L: l, R: r}
	}
}

func (o Or) isDNF() bool {
	return o.L.isDNF() && o.R.isDNF()
}

func (o Or) deMorgan() Set {
	return Not{And{
		L: Not{o.L},
		R: Not{o.R},
	}}
}

func (o Or) dist() Or {
	l, r := o.L, o.R
	lo, lok := l.(Or)
	ro, rok := r.(Or)
	// (la.l | la.r) | (a.r)
	if lok && !rok {
		return Or{
			L: Or{L: lo.L, R: r},
			R: Or{L: lo.R, R: r},
		}
	}
	// a.l | (ra.l | ra.r )
	if rok && !lok {
		return Or{
			L: Or{l, ro.L},
			R: Or{l, ro.R},
		}
	}
	return o
}
