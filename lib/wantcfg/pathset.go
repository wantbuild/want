package wantcfg

import (
	"fmt"
	"strings"

	"go.brendoncarroll.net/exp/slices2"
)

// PathSet represents a set of paths
type PathSet struct {
	Single    *string   `json:"single,omitempty"`
	Prefix    *string   `json:"prefix,omitempty"`
	Suffix    *string   `json:"suffix,omitempty"`
	Not       *PathSet  `json:"not,omitempty"`
	Union     []PathSet `json:"union,omitempty"`
	Intersect []PathSet `json:"intersect,omitempty"`
}

func (ps PathSet) String() string {
	// TODO: this is duplicated
	switch {
	case ps.Single != nil:
		return fmt.Sprintf("%q", *ps.Single)
	case ps.Prefix != nil:
		return fmt.Sprintf("%q*", *ps.Prefix)
	case ps.Suffix != nil:
		return fmt.Sprintf("*%q", *ps.Suffix)

	case ps.Not != nil:
		return "!" + ps.Not.String()

	case ps.Intersect != nil:
		parts := slices2.Map(ps.Intersect, func(x PathSet) string { return x.String() })
		return "(" + strings.Join(parts, " & ") + ")"
	case ps.Union != nil:
		parts := slices2.Map(ps.Union, func(x PathSet) string { return x.String() })
		return "(" + strings.Join(parts, " | ") + ")"

	default:
		return "{empty}"
	}
}

// Single returns a PathSet containing the single path x
func Single(x string) PathSet {
	return PathSet{Single: &x}
}

// Prefix returns a PathSet containing all paths with prefix x
func Prefix(x string) PathSet {
	return PathSet{Prefix: &x}
}

// Suffix returns a PathSet containing all paths with suffix x
func Suffix(x string) PathSet {
	return PathSet{Suffix: &x}
}

// Union returns the union of n PathSets
func Union(xs ...PathSet) PathSet {
	return PathSet{Union: xs}
}

// Intersection returns the intersection of n PathSets
func Intersect(xs ...PathSet) PathSet {
	return PathSet{Intersect: xs}
}

// Not returns a PathSet containing all paths not in x
func Not(x PathSet) PathSet {
	return PathSet{Not: &x}
}

// DirPath returns the set of paths contained in a directory at x
func DirPath(x string) PathSet {
	var children PathSet
	if x == "" {
		children = Prefix("")
	} else {
		children = Prefix(x + "/")
	}
	return Union(Single(x), children)
}

func Subtract(l, r PathSet) PathSet {
	return Intersect(l, Not(r))
}
