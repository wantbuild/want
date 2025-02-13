package stringsets

import (
	"regexp"

	"go.brendoncarroll.net/exp/maybe"
)

type Set interface {
	String() string
	Contains(x string) bool

	intersects(Set) maybe.Maybe[bool]
	superset(Set) maybe.Maybe[bool]
	simplify() Set
	isDNF() bool
}

func Intersects(a Set, b Set) bool {
	if yes := intersects(a, b); yes.Ok {
		return yes.X
	}
	return !Equals(And{a, b}, Empty{})
}

func intersects(a, b Set) maybe.Maybe[bool] {
	if yes := a.intersects(b); yes.Ok {
		return yes
	}
	if yes := b.intersects(a); yes.Ok {
		return yes
	}
	return maybe.Nothing[bool]()
}

func Superset(super, sub Set) bool {
	if yes := superset(super, sub); yes.Ok {
		return yes.X
	}
	subtract := Subtract(sub, super)
	return Equals(subtract, Empty{})
}

func superset(a, b Set) maybe.Maybe[bool] {
	if yes := a.superset(b); yes.Ok {
		return yes
	}
	return maybe.Nothing[bool]()
}

func Subset(sub, super Set) bool {
	return Superset(super, sub)
}

func Intersection(xs ...Set) Set {
	l := len(xs)
	switch l {
	case 0:
		return Top{}
	case 1:
		return xs[0]
	case 2:
		return And{xs[0], xs[1]}
	default:
		return And{
			Intersection(xs[:l/2]...),
			Intersection(xs[l/2:]...),
		}
	}
}

func Union(xs ...Set) Set {
	l := len(xs)
	switch l {
	case 0:
		return Empty{}
	case 1:
		return xs[0]
	case 2:
		return Or{xs[0], xs[1]}
	default:
		return Or{
			Union(xs[:l/2]...),
			Union(xs[l/2:]...),
		}
	}
}

// Subtract a - b
func Subtract(a, b Set) Set {
	return And{a, Not{b}}
}

type Top struct{}

func (Top) Contains(string) bool             { return true }
func (Top) Regexp() *regexp.Regexp           { return regexp.MustCompile(".*") }
func (Top) Complement() Set                  { return Empty{} }
func (Top) intersects(Set) maybe.Maybe[bool] { return maybe.Just(true) }
func (Top) superset(Set) maybe.Maybe[bool]   { return maybe.Just(true) }
func (Top) String() string                   { return "𝐔" }
func (Top) simplify() Set                    { return Top{} }
func (Top) isDNF() bool                      { return true }

type Empty struct{}

func (Empty) Contains(string) bool             { return false }
func (Empty) Regexp() *regexp.Regexp           { return regexp.MustCompile(`a^`) }
func (Empty) Complement() Set                  { return Top{} }
func (Empty) intersects(Set) maybe.Maybe[bool] { return maybe.Just(false) }
func (Empty) superset(x Set) maybe.Maybe[bool] { return maybe.Just(x == Empty{}) }
func (Empty) String() string                   { return "∅" }
func (Empty) simplify() Set                    { return Empty{} }
func (Empty) isDNF() bool                      { return true }

func isTrue(x maybe.Maybe[bool]) bool {
	return x.Ok && x.X
}

func isFalse(x maybe.Maybe[bool]) bool {
	return x.Ok && !x.X
}
