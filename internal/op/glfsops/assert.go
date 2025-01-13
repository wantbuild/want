package glfsops

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/exp/streams"
	"go.brendoncarroll.net/state/cadata"
)

// Assertions is a set of assertions, it is the input to assert not including
// the object to check.
type Assertions struct {
	SubsetOf *glfs.Ref
}

func GetAssertions(ctx context.Context, s cadata.Getter, ref glfs.Ref) (*Assertions, error) {
	tree, err := glfs.GetTree(ctx, s, ref)
	if err != nil {
		return nil, err
	}
	var ret Assertions
	if ent := tree.Lookup("subsetOf"); ent != nil {
		ret.SubsetOf = &ent.Ref
	}
	return &ret, nil
}

func PostAssertions(ctx context.Context, s cadata.Poster, x Assertions) (*glfs.Ref, error) {
	var ents []glfs.TreeEntry
	if x.SubsetOf != nil {
		ents = append(ents, glfs.TreeEntry{
			Name:     "subsetOf",
			FileMode: 0o777,
			Ref:      *x.SubsetOf,
		})
	}
	return glfs.PostTreeEntries(ctx, s, ents)
}

func Assert(ctx context.Context, s cadata.GetPoster, input glfs.Ref) (*glfs.Ref, error) {
	ag := glfs.NewAgent()
	xRef, err := glfs.GetAtPath(ctx, s, input, "x")
	if err != nil {
		return nil, err
	}
	tree, err := glfs.GetTree(ctx, s, input)
	if err != nil {
		return nil, err
	}
	var msg string
	if ent := tree.Lookup("message"); ent != nil {
		data, err := ag.GetBlobBytes(ctx, s, ent.Ref, 1e6)
		if err != nil {
			return nil, err
		}
		msg = string(data)
	}

	// assert checks
	as, err := GetAssertions(ctx, s, input)
	if err != nil {
		return nil, err
	}
	err = CheckAssertions(ctx, s, *xRef, *as)
	// add error message if it exists
	if err != nil && msg != "" {
		err = fmt.Errorf("assert failed: msg=%v, cause=%w", msg, err)
	}
	return &input, err
}

// CheckAssertions
func CheckAssertions(ctx context.Context, s cadata.Getter, x glfs.Ref, as Assertions) (err error) {
	ag := glfs.NewAgent()
	if as.SubsetOf != nil {
		err = errors.Join(err, assertSubsetOf(ctx, ag, s, x, *as.SubsetOf))
	}
	return err
}

func assertSubsetOf(ctx context.Context, ag *glfs.Agent, s cadata.Getter, x, subsetOf glfs.Ref) error {
	if x.Type != subsetOf.Type {
		return fmt.Errorf("assert: mismatched types %q != %q ", x.Type, subsetOf.Type)
	}
	if x.Type == glfs.TypeTree {
		treeIt, err := ag.NewTreeReader(s, subsetOf)
		if err != nil {
			return err
		}
		return streams.ForEach(ctx, treeIt, func(ent glfs.TreeEntry) error {
			x2, err := ag.GetAtPath(ctx, s, x, ent.Name)
			if err != nil {
				return err
			}
			return assertSubsetOf(ctx, ag, s, *x2, ent.Ref)
		})
	}
	rx, err := ag.GetTyped(ctx, s, x.Type, x)
	if err != nil {
		return err
	}
	rso, err := ag.GetTyped(ctx, s, x.Type, subsetOf)
	if err != nil {
		return err
	}
	return assertStreamsEqual(rx, rso)
}

func assertStreamsEqual(a, b io.Reader) error {
	bufa := bufio.NewReader(a)
	bufb := bufio.NewReader(b)

	var aEOF, bEOF bool
	var count int
	for !(aEOF || bEOF) {
		abyte, err := bufa.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				aEOF = true
			} else {
				return err
			}
		}
		bbyte, err := bufb.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				bEOF = true
			} else {
				return err
			}
		}
		if abyte != bbyte {
			return fmt.Errorf("streams unequal at byte %d, %q != %q", count, abyte, bbyte)
		}
		count++
	}
	if aEOF != bEOF {
		return errors.New("streams of unequal length")
	}
	return nil
}
