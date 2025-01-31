package stringsets

import "strings"

func SetFromGlob(g string) Set {
	parts := strings.Split(g, "*")
	if len(parts) == 1 {
		return Unit(g)
	}
	if len(parts) == 2 {
		return And{
			Prefix(parts[1]),
			Suffix(parts[2]),
		}
	}
	panic("don't support more than 1 wildcard yet")
}

func BoundingPrefix(x Set) string {
	x = Simplify(x)
	switch x := x.(type) {
	case Unit:
		return string(x)
	case Prefix:
		return string(x)
	case Suffix:
		return ""
	case Top:
		return ""
	case Empty:
		panic("empty key space has no prefix")
	case Not:
		return ""
	case Or:
		lp := BoundingPrefix(x.L)
		rp := BoundingPrefix(x.R)
		return longestCommonPrefix(lp, rp)
	case And:
		lp := BoundingPrefix(x.L)
		rp := BoundingPrefix(x.R)
		if !(strings.HasPrefix(lp, rp) && strings.HasPrefix(rp, lp)) {
			panic("and statement was not completely simplified: " + x.String())
		}
		return longestCommonPrefix(lp, rp)
	default:
		panic(x)
	}
}

func longestCommonPrefix(xs ...string) string {
	if len(xs) < 1 {
		return ""
	}
	lcp := xs[0]
	for _, x := range xs[1:] {
		lcp = lcp[:commonPrefixLen(lcp, x)]
		if len(lcp) == 0 {
			break
		}
	}
	return lcp
}

// commonPrefixLen returns the index of the first different character.
func commonPrefixLen(a, b string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return min(len(a), len(b))
}

func MapPrependPrefix(x Set, p Prefix) Set {
	switch x := x.(type) {
	case Or:
		return Or{L: MapPrependPrefix(x.L, p), R: MapPrependPrefix(x.R, p)}
	case And:
		return And{L: MapPrependPrefix(x.L, p), R: MapPrependPrefix(x.R, p)}
	case Prefix:
		return p + x
	default:
		return And{L: x, R: p}.simplify()
	}
}
