package wantcfg

import (
	"fmt"
	"io/fs"

	"blobcache.io/glfs"
)

type Tree = []TreeEntry

type TreeEntry struct {
	Name  string      `json:"name"`
	Mode  fs.FileMode `json:"mode"`
	Value Expr        `json:"value"`
}

type Expr struct {
	Blob      *string    `json:"blob,omitempty"`
	Tree      Tree       `json:"tree,omitempty"`
	Ref       *glfs.Ref  `json:"ref,omitempty"`
	Compute   *Compute   `json:"compute,omitempty"`
	Selection *Selection `json:"selection,omitempty"`
}

func Blob(x string) Expr {
	return Expr{Blob: &x}
}

func Literal(x glfs.Ref) Expr {
	return Expr{Ref: &x}
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
		return fmt.Sprintf("{Source: %v, Query: %v}", n.Selection.Source, n.Selection.Query)
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

type Source struct {
	Module     string `json:"module"`
	Derived    bool   `json:"derived"`
	CallerPath string `json:"callerPath"`
}

type Selection struct {
	// Source is the domain to query.
	// It must be either GROUND or DERIVED.
	Source Source `json:"source"`
	// Query is a PathSet which will be used to filter Source.
	Query PathSet `json:"query"`

	// Pick will apply a glfs.Pick operation after Query has been applied to Source
	Pick string `json:"pick,omitempty"`
}
