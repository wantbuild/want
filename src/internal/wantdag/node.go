package wantdag

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/blobcache/glfs"
	"wantbuild.io/want/src/wantjob"
)

const (
	MaxNodeInputs  = 64
	MaxInputKeyLen = 4096
	InputFileMode  = fs.FileMode(0o777)
)

type OpName = wantjob.OpName

// NodeID identifies a Node within a Graph
type NodeID uint64

// Node is either a fact or a derived value in a computation graph.
type Node struct {
	Value *glfs.Ref

	Op     OpName
	Inputs []NodeInput
}

func (n *Node) IsFact() bool {
	return n.Value != nil
}

func (n *Node) IsDerived() bool {
	return n.Op != ""
}

func (n *Node) Validate() error {
	if n.IsFact() && n.IsDerived() {
		return errors.New("ambiguous node")
	}
	if !n.IsFact() && !n.IsDerived() {
		return errors.New("empty node")
	}
	if n.IsDerived() {
		if err := CheckNodeInputs(n.Inputs); err != nil {
			return err
		}
	}
	if n.IsFact() {
		if n.Value.CID.IsZero() {
			return errors.New("zero value for node ref")
		}
	}
	return nil
}

func (n *Node) Deps() []NodeID {
	deps := []NodeID{}
	for _, ni := range n.Inputs {
		deps = append(deps, ni.Node)
	}
	return deps
}

type NodeInput struct {
	Name string
	Node NodeID
}

func CheckNodeInputs(ins []NodeInput) error {
	for i := 1; i < len(ins); i++ {
		if ins[i].Name < ins[i-1].Name {
			return fmt.Errorf("node inputs are unsorted (%v after %v)", ins[i], ins[i-1])
		}
		if ins[i].Name == ins[i-1].Name {
			return fmt.Errorf("path %q appears multiple times", ins[i].Name)
		}
	}
	for _, in := range ins {
		if strings.Contains(in.Name, "/") {
			return fmt.Errorf("input names must only contain one path element")
		}
	}
	return nil
}
