package assertops

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/exp/streams"
	"go.brendoncarroll.net/state/cadata"
)

const MaxPathLen = 4096

type AssertTask struct {
	X glfs.Ref

	Msg string
	Assertions
}

// Assertions is a set of assertions, it is the input to assert not including
// the object to check.
type Assertions struct {
	SubsetOf   *glfs.Ref
	PathExists *string
}

func PostAssertTask(ctx context.Context, s cadata.PostExister, x AssertTask) (*glfs.Ref, error) {
	var ents []glfs.TreeEntry
	ents = append(ents, glfs.TreeEntry{
		Name:     "x",
		FileMode: 0o777,
		Ref:      x.X,
	})
	if x.SubsetOf != nil {
		ents = append(ents, glfs.TreeEntry{
			Name:     "subsetOf",
			FileMode: 0o777,
			Ref:      *x.SubsetOf,
		})
	}
	if x.PathExists != nil {
		ref, err := glfs.PostBlob(ctx, s, strings.NewReader(*x.PathExists))
		if err != nil {
			return nil, err
		}
		ents = append(ents, glfs.TreeEntry{
			Name:     "pathExists",
			FileMode: 0o777,
			Ref:      *ref,
		})
	}
	return glfs.PostTreeSlice(ctx, s, ents)
}

func GetAssertTask(ctx context.Context, s cadata.Getter, x glfs.Ref) (*AssertTask, error) {
	tree, err := glfs.GetTreeSlice(ctx, s, x, 1e6)
	if err != nil {
		return nil, err
	}
	var ret AssertTask
	if ent := glfs.Lookup(tree, "x"); ent != nil {
		ret.X = ent.Ref
	} else {
		return nil, fmt.Errorf("assert task missing x")
	}
	if ent := glfs.Lookup(tree, "message"); ent != nil {
		data, err := glfs.GetBlobBytes(ctx, s, ent.Ref, 1e6)
		if err != nil {
			return nil, err
		}
		ret.Msg = string(data)
	}

	if ent := glfs.Lookup(tree, "subsetOf"); ent != nil {
		ret.SubsetOf = &ent.Ref
	}
	if ent := glfs.Lookup(tree, "pathExists"); ent != nil {
		data, err := glfs.GetBlobBytes(ctx, s, ent.Ref, MaxPathLen)
		if err != nil {
			return nil, err
		}
		p := string(data)
		ret.PathExists = &p
	}
	return &ret, nil
}

func AssertAll(ctx context.Context, dst cadata.Poster, src cadata.Getter, task AssertTask) (*glfs.Ref, error) {
	// assert checks
	err := checkAssertions(ctx, src, task.X, task.Assertions)
	// add error message if it exists
	if err != nil && task.Msg != "" {
		err = fmt.Errorf("assert failed: msg=%v, cause=%w", task.Msg, err)
	}
	return &task.X, err
}

func checkAssertions(ctx context.Context, s cadata.Getter, x glfs.Ref, as Assertions) (err error) {
	ag := glfs.NewAgent()
	if as.SubsetOf != nil {
		err = errors.Join(err, assertSubsetOf(ctx, ag, s, x, *as.SubsetOf))
	}
	if as.PathExists != nil {
		err = errors.Join(err, assertPathExists(ctx, ag, s, x, *as.PathExists))
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

func assertPathExists(ctx context.Context, ag *glfs.Agent, s cadata.Getter, x glfs.Ref, p string) error {
	_, err := ag.GetAtPath(ctx, s, x, p)
	return err
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
