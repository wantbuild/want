package glfsops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"blobcache.io/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/internal/stringsets"
	"wantbuild.io/want/src/internal/wantdag"
	"wantbuild.io/want/src/wantcfg"
	"wantbuild.io/want/src/wantjob"
)

type (
	OpName    = wantjob.OpName
	NodeID    = wantdag.NodeID
	NodeInput = wantdag.NodeInput
)

const (
	OpMerge         = OpName("merge")
	OpPick          = OpName("pick")
	OpPlace         = OpName("place")
	OpPassthrough   = OpName("pass")
	OpFilterPathSet = OpName("filterPathSet")
	OpChmod         = OpName("chmod")
	OpDiff          = OpName("diff")
)

const MaxPathLen = 4096

var ops = map[OpName]Operator{
	OpMerge:         Merge,
	OpPick:          Pick,
	OpPlace:         Place,
	OpPassthrough:   Passthrough,
	OpFilterPathSet: FilterPathSet,
	OpChmod:         Chmod,
	OpDiff:          Diff,
}

type Operator func(ctx context.Context, dst cadata.PostExister, src cadata.Getter, x glfs.Ref) (*glfs.Ref, error)

func Merge(ctx context.Context, dst cadata.PostExister, src cadata.Getter, inputsRef glfs.Ref) (*glfs.Ref, error) {
	t, err := glfs.GetTreeSlice(ctx, src, inputsRef, 1e6)
	if err != nil {
		return nil, err
	}
	layers := []glfs.Ref{}
	for _, ent := range t {
		layers = append(layers, ent.Ref)
	}
	if len(layers) == 0 {
		return nil, errors.New("cannot merge 0 layers")
	}
	return glfs.Merge(ctx, dst, src, layers...)
}

func Pick(ctx context.Context, _ cadata.PostExister, src cadata.Getter, inputRef glfs.Ref) (*glfs.Ref, error) {
	inputTree, err := glfs.GetTreeSlice(ctx, src, inputRef, 1e6)
	if err != nil {
		return nil, err
	}
	xent := glfs.Lookup(inputTree, "x")
	if xent == nil {
		return nil, errors.New("no target")
	}
	pathEnt := glfs.Lookup(inputTree, "path")
	if pathEnt == nil {
		return nil, errors.New("no path")
	}
	pathBytes, err := glfs.GetBlobBytes(ctx, src, pathEnt.Ref, MaxPathLen)
	if err != nil {
		return nil, fmt.Errorf("pick: while reading path %w", err)
	}
	return glfs.GetAtPath(ctx, src, xent.Ref, string(pathBytes))
}

func Place(ctx context.Context, dst cadata.PostExister, src cadata.Getter, inputRef glfs.Ref) (*glfs.Ref, error) {
	inputTree, err := glfs.GetTreeSlice(ctx, src, inputRef, 1e6)
	if err != nil {
		return nil, err
	}
	xent := glfs.Lookup(inputTree, "x")
	if xent == nil {
		return nil, errors.New("no target")
	}
	pathEnt := glfs.Lookup(inputTree, "path")
	if pathEnt == nil {
		return nil, errors.New("no path")
	}
	pathBytes, err := glfs.GetBlobBytes(ctx, src, pathEnt.Ref, MaxPathLen)
	if err != nil {
		return nil, fmt.Errorf("place: while reading path %w", err)
	}
	if err := glfstasks.FastSync(ctx, dst, src, xent.Ref); err != nil {
		return nil, err
	}
	return glfs.PostTreeSlice(ctx, dst, []glfs.TreeEntry{
		{Name: string(pathBytes), FileMode: 0o777, Ref: xent.Ref},
	})
}

func Passthrough(ctx context.Context, dst cadata.PostExister, src cadata.Getter, inputRef glfs.Ref) (*glfs.Ref, error) {
	if err := glfstasks.FastSync(ctx, dst, src, inputRef); err != nil {
		return nil, err
	}
	return &inputRef, nil
}

func FilterPathSet(ctx context.Context, dst cadata.PostExister, src cadata.Getter, inputRef glfs.Ref) (*glfs.Ref, error) {
	root, err := glfs.GetTreeSlice(ctx, src, inputRef, 1e6)
	if err != nil {
		return nil, err
	}
	targetEnt := glfs.Lookup(root, "x")
	if targetEnt == nil {
		return nil, errors.New("missing target")
	}
	filterEnt := glfs.Lookup(root, "filter")
	if filterEnt == nil {
		return nil, errors.New("missing filter")
	}
	data, err := glfs.GetBlobBytes(ctx, src, filterEnt.Ref, 1e6)
	if err != nil {
		return nil, fmt.Errorf("filter must be blob %w", err)
	}
	var q wantcfg.PathSet
	if err := json.Unmarshal(data, &q); err != nil {
		return nil, fmt.Errorf("parsing pathset: %w", err)
	}
	ss := stringsets.FromPathSet(q)
	return glfs.FilterPaths(ctx, dst, src, targetEnt.Ref, func(x string) bool {
		return ss.Contains(x)
	})
}

func Chmod(ctx context.Context, dst cadata.PostExister, src cadata.Getter, inputRef glfs.Ref) (*glfs.Ref, error) {
	inputTree, err := glfs.GetTreeSlice(ctx, src, inputRef, 1e6)
	if err != nil {
		return nil, err
	}
	xEnt := glfs.Lookup(inputTree, "x")
	if xEnt == nil {
		return nil, NewErrInvalidInput(inputRef, "set-permissions requires input 'x'")
	}
	pathEnt := glfs.Lookup(inputTree, "path")
	if pathEnt == nil {
		return nil, NewErrInvalidInput(inputRef, "set-permissions requires input 'path'")
	}
	pathData, err := glfs.GetBlobBytes(ctx, src, pathEnt.Ref, MaxPathLen)
	if err != nil {
		return nil, err
	}
	// TODO: support changing permissions to any value.
	p := string(bytes.Trim(bytes.TrimSpace(pathData), "/"))
	return glfs.MapEntryAt(ctx, dst, src, xEnt.Ref, p, func(ent glfs.TreeEntry) (*glfs.TreeEntry, error) {
		ent.FileMode |= 0o111
		return &ent, nil
	})
}

func Diff(ctx context.Context, dst cadata.PostExister, src cadata.Getter, inputRef glfs.Ref) (*glfs.Ref, error) {
	ag := glfs.NewAgent()
	left, err := ag.GetAtPath(ctx, src, inputRef, "left")
	if err != nil {
		return nil, err
	}
	right, err := ag.GetAtPath(ctx, src, inputRef, "right")
	if err != nil {
		return nil, err
	}
	diff, err := ag.Compare(ctx, dst, src, *left, *right)
	if err != nil {
		return nil, err
	}
	ents := make([]glfs.TreeEntry, 0, 3)
	if diff.Left != nil {
		ref := *diff.Left
		ents = append(ents, makeTreeEntry("left", ref))
	}
	if diff.Right != nil {
		ents = append(ents, makeTreeEntry("right", *diff.Right))
	}
	if diff.Both != nil {
		ents = append(ents, makeTreeEntry("both", *diff.Both))
	}
	return ag.PostTreeSlice(ctx, dst, ents)
}

func makeTreeEntry(name string, ref glfs.Ref) glfs.TreeEntry {
	return glfs.TreeEntry{
		FileMode: getFileMode(ref),
		Name:     name,
		Ref:      ref,
	}
}

func getFileMode(x glfs.Ref) os.FileMode {
	mode := wantdag.InputFileMode
	if x.Type == glfs.TypeTree {
		mode |= fs.ModeDir
		return 0
	}
	return mode
}

type AssertChecks struct {
	SubsetOf *wantdag.NodeID
	Message  string
}

type ErrInvalidInput struct {
	Input glfs.Ref
	Msg   string
}

func NewErrInvalidInput(input glfs.Ref, msg string) ErrInvalidInput {
	return ErrInvalidInput{Input: input, Msg: msg}
}

func (e ErrInvalidInput) Error() string {
	return fmt.Sprintf("invalid input. input=%v msg=%v", e.Input, e.Msg)
}
