package glfsops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/wantdag"
	"wantbuild.io/want/src/wantjob"
)

type (
	OpName    = wantjob.OpName
	NodeID    = wantdag.NodeID
	NodeInput = wantdag.NodeInput
)

const (
	OpMerge       = OpName("merge")
	OpPick        = OpName("pick")
	OpPlace       = OpName("place")
	OpPassthrough = OpName("pass")
	OpFilter      = OpName("filter")
	OpChmod       = OpName("chmod")
	OpDiff        = OpName("diff")
)

const MaxPathLen = 4096

var ops = map[OpName]Operator{
	OpMerge:       Merge,
	OpPick:        Pick,
	OpPlace:       Place,
	OpPassthrough: Passthrough,
	OpFilter:      Filter,
	OpChmod:       Chmod,
	OpDiff:        Diff,
}

type Operator func(ctx context.Context, s cadata.GetPoster, x glfs.Ref) (*glfs.Ref, error)

func Merge(ctx context.Context, s cadata.GetPoster, inputsRef glfs.Ref) (*glfs.Ref, error) {
	t, err := glfs.GetTree(ctx, s, inputsRef)
	if err != nil {
		return nil, err
	}
	layers := []glfs.Ref{}
	for _, ent := range t.Entries {
		layers = append(layers, ent.Ref)
	}
	if len(layers) == 0 {
		return nil, errors.New("cannot merge 0 layers")
	}
	return glfs.Merge(ctx, s, layers...)
}

func Pick(ctx context.Context, s cadata.GetPoster, inputRef glfs.Ref) (*glfs.Ref, error) {
	inputTree, err := glfs.GetTree(ctx, s, inputRef)
	if err != nil {
		return nil, err
	}
	xent := inputTree.Lookup("x")
	if xent == nil {
		return nil, errors.New("no target")
	}
	pathEnt := inputTree.Lookup("path")
	if pathEnt == nil {
		return nil, errors.New("no path")
	}
	pathBytes, err := glfs.GetBlobBytes(ctx, s, pathEnt.Ref, MaxPathLen)
	if err != nil {
		return nil, fmt.Errorf("pick: while reading path %w", err)
	}
	return glfs.GetAtPath(ctx, s, xent.Ref, string(pathBytes))
}

func Place(ctx context.Context, s cadata.GetPoster, inputRef glfs.Ref) (*glfs.Ref, error) {
	inputTree, err := glfs.GetTree(ctx, s, inputRef)
	if err != nil {
		return nil, err
	}
	xent := inputTree.Lookup("x")
	if xent == nil {
		return nil, errors.New("no target")
	}
	pathEnt := inputTree.Lookup("path")
	if pathEnt == nil {
		return nil, errors.New("no path")
	}
	pathBytes, err := glfs.GetBlobBytes(ctx, s, pathEnt.Ref, MaxPathLen)
	if err != nil {
		return nil, fmt.Errorf("place: while reading path %w", err)
	}
	return glfs.PostTreeEntries(ctx, s, []glfs.TreeEntry{
		{Name: string(pathBytes), FileMode: 0o777, Ref: xent.Ref},
	})
}

func Passthrough(ctx context.Context, s cadata.GetPoster, inputRef glfs.Ref) (*glfs.Ref, error) {
	return &inputRef, nil
}

func Filter(ctx context.Context, s cadata.GetPoster, inputRef glfs.Ref) (*glfs.Ref, error) {
	root, err := glfs.GetTree(ctx, s, inputRef)
	if err != nil {
		return nil, err
	}
	targetEnt := root.Lookup("x")
	if targetEnt == nil {
		return nil, errors.New("missing target")
	}
	filterEnt := root.Lookup("filter")
	if filterEnt == nil {
		return nil, errors.New("missing filter")
	}
	data, err := glfs.GetBlobBytes(ctx, s, filterEnt.Ref, 1e6)
	if err != nil {
		return nil, fmt.Errorf("filter must be blob %w", err)
	}
	re, err := regexp.Compile(string(data))
	if err != nil {
		return nil, err
	}
	return glfs.FilterPaths(ctx, s, targetEnt.Ref, func(x string) bool {
		return re.MatchString(x)
	})
}

func Chmod(ctx context.Context, s cadata.GetPoster, inputRef glfs.Ref) (*glfs.Ref, error) {
	inputTree, err := glfs.GetTree(ctx, s, inputRef)
	if err != nil {
		return nil, err
	}
	xEnt := inputTree.Lookup("x")
	if xEnt == nil {
		return nil, NewErrInvalidInput(inputRef, "set-permissions requires input 'x'")
	}
	pathEnt := inputTree.Lookup("path")
	if pathEnt == nil {
		return nil, NewErrInvalidInput(inputRef, "set-permissions requires input 'path'")
	}
	pathData, err := glfs.GetBlobBytes(ctx, s, pathEnt.Ref, MaxPathLen)
	if err != nil {
		return nil, err
	}
	// TODO: support changing permissions to any value.
	p := string(bytes.Trim(bytes.TrimSpace(pathData), "/"))
	return glfs.MapEntryAt(ctx, s, xEnt.Ref, p, func(ent glfs.TreeEntry) (*glfs.TreeEntry, error) {
		ent.FileMode |= 0o111
		return &ent, nil
	})
}

func Diff(ctx context.Context, s cadata.GetPoster, inputRef glfs.Ref) (*glfs.Ref, error) {
	ag := glfs.NewAgent()
	left, err := ag.GetAtPath(ctx, s, inputRef, "left")
	if err != nil {
		return nil, err
	}
	right, err := ag.GetAtPath(ctx, s, inputRef, "right")
	if err != nil {
		return nil, err
	}
	diff, err := ag.Compare(ctx, s, *left, *right)
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
	return ag.PostTreeEntries(ctx, s, ents)
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
