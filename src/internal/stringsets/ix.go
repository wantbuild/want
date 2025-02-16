package stringsets

import (
	"regexp"
	"strings"

	"go.brendoncarroll.net/exp/maybe"
)

type Prefix string

func (p Prefix) Contains(x string) bool {
	return strings.HasPrefix(x, string(p))
}

func (p Prefix) String() string {
	return ToPathSet(p).String()
}

func (p Prefix) intersects(x Set) maybe.Maybe[bool] {
	y := false
	switch x := x.(type) {
	case Unit:
		y = strings.HasPrefix(string(x), string(p))
	case Prefix:
		y = strings.HasPrefix(string(x), string(p)) || strings.HasPrefix(string(p), string(x))
	case Suffix:
		y = true
	default:
		return maybe.Nothing[bool]()
	}
	return maybe.Just(y)
}

func (p Prefix) superset(x Set) maybe.Maybe[bool] {
	y := false
	switch x := x.(type) {
	case Empty:
		y = true
	case Top:
		y = p == ""
	case Unit:
		y = strings.HasPrefix(string(x), string(p))
	case Prefix:
		y = strings.HasPrefix(string(x), string(p))
	case Suffix:
		y = p == "" && x == ""
	default:
		return maybe.Nothing[bool]()
	}
	return maybe.Just(y)
}

func (p Prefix) simplify() Set {
	return p
}

func (p Prefix) isDNF() bool {
	return true
}

type Suffix string

func (s Suffix) Contains(x string) bool {
	return strings.HasSuffix(x, string(s))
}

func (s Suffix) Regexp() *regexp.Regexp {
	return regexp.MustCompile(regexp.QuoteMeta(string(s)) + "$")
}

func (s Suffix) intersects(x Set) maybe.Maybe[bool] {
	y := false
	switch x := x.(type) {
	case Unit:
		y = strings.HasSuffix(string(x), string(s))
	case Prefix:
		y = true
	case Suffix:
		y = strings.HasSuffix(string(x), string(s)) || strings.HasSuffix(string(s), string(x))
	default:
		return maybe.Nothing[bool]()
	}
	return maybe.Just(y)
}

func (s Suffix) superset(x Set) maybe.Maybe[bool] {
	y := false
	switch x := x.(type) {
	case Empty:
		y = true
	case Top:
		y = s == ""
	case Unit:
		y = strings.HasSuffix(string(x), string(s))
	case Prefix:
		y = s == "" && x == ""
	case Suffix:
		y = strings.HasSuffix(string(x), string(s))
	default:
		return maybe.Nothing[bool]()
	}
	return maybe.Just(y)
}

func (s Suffix) simplify() Set {
	return s
}

func (s Suffix) isDNF() bool {
	return true
}

func (s Suffix) String() string {
	return "*" + string(s)
}
