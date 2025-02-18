package wantc

import (
	"context"
	"fmt"
	"strings"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/exp/slices2"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/op/glfsops"
	"wantbuild.io/want/src/internal/stringsets"
	"wantbuild.io/want/src/wantcfg"
	"wantbuild.io/want/src/wantjob"
)

// BoundingPrefix returns the longest prefix that supersets x
func BoundingPrefix(x wantcfg.PathSet) string {
	return stringsets.BoundingPrefix(stringsets.FromPathSet(x))
}

// Intersects returns true iff the 2 path sets intsersect
func Intersects(a, b wantcfg.PathSet) bool {
	return stringsets.Intersects(stringsets.FromPathSet(a), stringsets.FromPathSet(b))
}

// Select performs the selection logic on a known filesystem root.
func Select(ctx context.Context, dst cadata.Store, src cadata.Getter, root glfs.Ref, q wantcfg.PathSet) (*glfs.Ref, error) {
	strset := stringsets.FromPathSet(q)
	return glfs.FilterPaths(ctx, dst, src, root, func(p string) bool {
		return strset.Contains(p)
	})
}

func (c *Compiler) query(ctx context.Context, dst cadata.PostExister, vfs *VFS, ks stringsets.Set) (Expr, error) {
	vents := vfs.Get(ks)
	q := stringsets.ToPathSet(ks)
	var layers []Expr
	for _, vent := range vents {
		layer, err := c.placeAt(ctx, dst, vent.V, vent.PlaceAt)
		if err != nil {
			return nil, err
		}
		layer, err = c.filterExpr(ctx, dst, layer, q)
		if err != nil {
			return nil, err
		}
		layers = append(layers, layer)
	}
	return c.merge(layers)
}

func (c *Compiler) placeAt(ctx context.Context, dst cadata.PostExister, x Expr, p string) (Expr, error) {
	if p == "" {
		return x, nil
	}
	pathRef, err := c.glfs.PostBlob(ctx, dst, strings.NewReader(p))
	if err != nil {
		return nil, err
	}
	return &compute{
		Op: wantjob.OpName("glfs." + glfsops.OpPlace),
		Inputs: []computeInput{
			{To: "x", From: x},
			{To: "path", From: &value{*pathRef}},
		},
	}, nil
}

func (c *Compiler) merge(layers []Expr) (Expr, error) {
	var inputs []computeInput
	for i, layer := range layers {
		inputs = append(inputs, computeInput{
			To:   fmt.Sprintf("%08x", i),
			Mode: 0o777,
			From: layer,
		})
	}
	return &compute{
		Op:     wantjob.OpName("glfs.") + glfsops.OpMerge,
		Inputs: inputs,
	}, nil
}

func (c *Compiler) selectGround(cc *compileCtx, modID ModuleID, set stringsets.Set) (*value, error) {
	var modRef *glfs.Ref
	if modID == cc.ground.CID {
		modRef = &cc.ground
	} else {
		for _, sm := range cc.subMods {
			if NewModuleID(sm.root) == modID {
				modRef = &sm.root
				break
			}
		}
	}

	// TODO: look for the matching module
	if modRef == nil {
		return nil, fmt.Errorf("could not select from GROUND for module %v", modID)
	}
	ref, err := glfs.FilterPaths(cc.ctx, cc.dst, cc.src, cc.ground, func(p string) bool {
		return set.Contains(p)
	})
	if err != nil {
		return nil, err
	}
	return &value{ref: *ref}, nil
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
