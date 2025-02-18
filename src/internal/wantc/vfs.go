package wantc

import (
	"slices"
	"strings"

	"wantbuild.io/want/src/internal/stringsets"
)

type VFS struct {
	root vfsNode
}

func (v *VFS) Add(x VFSEntry) error {
	ents := v.Get(x.K)
	if len(ents) > 0 {
		return ErrConflict{Overlapping: []stringsets.Set{ents[0].K, x.K}}
	}
	n := v.root.Add(v.nodePath(x.K))
	n.ents = append(n.ents, x)
	return nil
}

func (v *VFS) Get(k stringsets.Set) (ret []VFSEntry) {
	if k == (stringsets.Empty{}) {
		panic(k)
	}
	n := v.root.Closest(v.nodePath(k))
	// get everything contained in the bounding prefix.
	n.ForEach(func(ent VFSEntry) bool {
		if stringsets.Intersects(ent.K, k) {
			ret = append(ret, ent)
		}
		return true
	})
	// look for other things in the parent path that could intersect with it
	for n = n.parent; n != nil; n = n.parent {
		for _, ent := range n.ents {
			if stringsets.Intersects(ent.K, k) {
				ret = append(ret, ent)
			}
		}
	}
	return ret
}

func (v *VFS) Delete(ks stringsets.Set) {
	for n := v.root.Closest(v.nodePath(ks)); n != nil; n = n.parent {
		n.ents = slices.DeleteFunc(n.ents, func(x VFSEntry) bool {
			return stringsets.Intersects(ks, x.K)
		})
	}
}

func (v *VFS) ForEach(fn func(VFSEntry) bool) bool {
	return v.root.ForEach(fn)
}

func (v *VFS) nodePath(ks stringsets.Set) []string {
	bp := stringsets.BoundingPrefix(ks)
	bp = strings.Trim(bp, "/")
	return strings.Split(bp, "/")
}

type VFSEntry struct {
	// K is the path space occupied by this entry.
	// It must not conflict with any other entries.
	K stringsets.Set
	// V is the value that occupies this space
	// It should be a layer expression, meaning that it evaluates to module level paths.
	V Expr
	// PlaceAt is where V should be placed within the submodule.
	// This should always be within the bounding prefix of K
	PlaceAt string

	IsSubmodule bool
	DefinedIn   string
	DefinedNum  int
}

type vfsNode struct {
	parent   *vfsNode
	children map[string]*vfsNode
	ents     []VFSEntry
}

// ForEach iterates over all of the entries in this node and all of its children.
func (n *vfsNode) ForEach(fn func(VFSEntry) bool) bool {
	if n == nil {
		return true
	}
	for _, child := range n.children {
		if !child.ForEach(fn) {
			return false
		}
	}
	for _, ent := range n.ents {
		if !fn(ent) {
			return false
		}
	}
	return true
}

func (n *vfsNode) Closest(p []string) *vfsNode {
	if n == nil || len(p) == 0 {
		return n
	}
	child := n.children[p[0]]
	if child == nil {
		return n
	}
	return child.Closest(p[1:])
}

func (n *vfsNode) Add(p []string) *vfsNode {
	if n == nil || len(p) == 0 {
		return n
	}
	k := p[0]
	if n.children == nil {
		n.children = make(map[string]*vfsNode)
	}
	if _, exists := n.children[k]; !exists {
		n.children[k] = &vfsNode{parent: n.parent}
	}
	return n.children[k].Add(p[1:])
}
