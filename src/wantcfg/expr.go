package wantcfg

import (
	"fmt"
	"io/fs"

	"github.com/blobcache/glfs"
)

type Tree = map[string]TreeEntry

type TreeEntry struct {
	Mode  fs.FileMode `json:"mode"`
	Value Expr        `json:"value"`
}

type Ref = glfs.Ref

type Expr struct {
	Blob      *string    `json:"blob,omitempty"`
	Tree      Tree       `json:"tree,omitempty"`
	Ref       *Ref       `json:"ref,omitempty"`
	Compute   *Compute   `json:"compute,omitempty"`
	Selection *Selection `json:"selection,omitempty"`
}

func (e Expr) IsValue() bool {
	return e.Blob != nil || e.Tree != nil
}

func (n Expr) String() string {
	switch {
	case n.Blob != nil:
		return fmt.Sprintf("{FileLiteral: %s}", *n.Blob)
	case n.Tree != nil:
		return fmt.Sprintf("{TreeLiteral: %v}", n.Tree)
	case n.Ref != nil:
		return fmt.Sprintf("{Ref: %v}", n.Ref)
	case n.Selection != nil:
		return fmt.Sprintf("{Source: %s, Query: %v}", n.Selection.Source, n.Selection.Query)
	case n.Compute != nil:
		c := *n.Compute
		return fmt.Sprintf("{Compute Op: %s, Inputs: %v}", c.Op, c.Inputs)
	default:
		return "(empty)"
	}
}

type Input struct {
	To   string      `json:"to"`
	From Expr        `json:"from"`
	Mode fs.FileMode `json:"mode"`
}

type Compute struct {
	Op     string  `json:"op"`
	Inputs []Input `json:"inputs"`
}

// Source is the tree to select from.
type Source string

const (
	// Fact is the Source for selecting fact data from the input tree
	Fact = Source("fact")
	// Derived is the Source for selecting derived data from the VFS
	Derived = Source("derived")
)

type Selection struct {
	Source Source  `json:"source"`
	Query  PathSet `json:"query"`
	Pick   string  `json:"pick"`

	AllowEmpty bool   `json:"allowEmpty"`
	AssertType string `json:"assertType,omitempty"`

	CallerPath string `json:"callerPath"`
}
