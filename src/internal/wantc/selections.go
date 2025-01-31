package wantc

import (
	"context"
	"path"
	"regexp"
	"strings"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/exp/slices2"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/op/glfsops"
	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/internal/stringsets"
	"wantbuild.io/want/src/wantcfg"
	"wantbuild.io/want/src/wantjob"
)

func BoundingPrefix(x wantcfg.PathSet) string {
	ss := SetFromQuery("", x)
	return stringsets.BoundingPrefix(ss)
}

func Intersects(a, b wantcfg.PathSet) bool {
	return stringsets.Intersects(SetFromQuery("", a), SetFromQuery("", b))
}

// Select performs the selection logic on a known filesystem root.
func Select(ctx context.Context, dst cadata.Store, src cadata.Getter, root glfs.Ref, q wantcfg.PathSet) (*glfs.Ref, error) {
	strset := SetFromQuery("", q)
	return glfs.FilterPaths(ctx, stores.Fork{W: dst, R: src}, root, func(p string) bool {
		return strset.Contains(p)
	})
}

func (c *Compiler) query(ctx context.Context, dst cadata.Store, vfs *VFS, ks stringsets.Set, pick string) (Expr, error) {
	edges := queryEdges(vfs, ks, pick)
	ni, err := c.flattenEdges(ctx, dst, edges)
	if err != nil {
		return nil, err
	}
	return &compute{
		Op:     wantjob.OpName("glfs.") + glfsops.OpPassthrough,
		Inputs: ni,
	}, nil
}

// flattenEdges takes a slice of Edges and produces an input set for input to a node.
// It will create any necessary intermediary nodes for metadata operations.
func (c *Compiler) flattenEdges(ctx context.Context, dst cadata.Store, xs []*edge) ([]computeInput, error) {
	var ys []computeInput
	for _, x := range xs {
		y := x.Expr
		if x.Subpath != "" {
			var err error
			y, err = c.pickExpr(ctx, dst, y, x.Subpath)
			if err != nil {
				return nil, err
			}
		}
		// Filter
		if x.Filter != nil {
			var err error
			y, err = c.filterExpr(ctx, dst, y, x.Filter)
			if err != nil {
				return nil, err
			}
		}

		ys = append(ys, computeInput{
			To:   x.Key,
			From: y,
			Mode: 0o777,
		})
	}
	return ys, nil
}

// An Edge represents an expression plus a transformation
// Applied to that expression
type edge struct {
	Key string

	Expr    Expr
	Subpath string
	Filter  *regexp.Regexp
}

// select_ returns a list of edges that populate the ks region of gs
// note: select can return 0 edges, but that should produce an error during planning.
func queryEdges(vfs *VFS, ks stringsets.Set, pick string) []*edge {
	if stringsets.Equals(ks, stringsets.Empty{}) {
		return nil
	}
	edges := []*edge{}

	ents := vfs.Get(ks)
	for _, ent := range ents {
		ks2 := ent.K
		bp2 := stringsets.BoundingPrefix(ks2)

		var subpath string
		var key string
		if strings.HasPrefix(pick, bp2) {
			subpath = strings.Trim(pick[len(bp2):], "/")
		} else {
			key = strings.Trim(bp2[len(pick):], "/")
		}

		switch ex := ent.V.(type) {
		case *selection:
			edges2 := queryEdges(vfs, ex.set, pick)
			for _, e := range edges2 {
				e.Key = path.Join(key, e.Key)
				edges = append(edges, e)
			}
		default:
			ed := &edge{
				Key: key,

				Expr:    ex,
				Subpath: subpath,
				Filter:  nil,
			}
			edges = append(edges, ed)
		}
	}
	return edges
}

func makePathSet(x stringsets.Set) wantcfg.PathSet {
	switch x := x.(type) {
	case stringsets.Unit:
		return wantcfg.Unit(string(x))
	case stringsets.Prefix:
		return wantcfg.Prefix(string(x))
	case stringsets.Suffix:
		return wantcfg.Suffix(string(x))

	case stringsets.Not:
		return wantcfg.Not(makePathSet(x.X))

	case stringsets.And:
		return wantcfg.Intersect(makePathSet(x.L), makePathSet(x.R))
	case stringsets.Or:
		return wantcfg.Union(makePathSet(x.L), makePathSet(x.R))
	default:
		panic(x)
	}
}

// SetFromQuery returns a string set for a query asked from from.
func SetFromQuery(from string, q wantcfg.PathSet) stringsets.Set {
	switch {
	case q.Unit != nil:
		return stringsets.Unit(PathFrom(from, *q.Unit))
	case q.Prefix != nil:
		return stringsets.Prefix(PathFrom(from, *q.Prefix))
	case q.Suffix != nil:
		return stringsets.Suffix(*q.Suffix)
	case q.Union != nil:
		xs := slices2.Map(q.Union, func(x wantcfg.PathSet) stringsets.Set {
			return SetFromQuery(from, x)
		})
		return stringsets.Simplify(stringsets.Union(xs...))
	case q.Intersect != nil:
		xs := slices2.Map(q.Intersect, func(x wantcfg.PathSet) stringsets.Set {
			return SetFromQuery(from, x)
		})
		return stringsets.Simplify(stringsets.Intersection(xs...))
	case q.Not != nil:
		return stringsets.Not{X: SetFromQuery(from, *q.Not)}
	default:
		return stringsets.Empty{}
	}
}
