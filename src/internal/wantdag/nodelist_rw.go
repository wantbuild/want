package wantdag

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"blobcache.io/glfs"
	"go.brendoncarroll.net/exp/streams"
	"go.brendoncarroll.net/state/cadata"
)

type NodeListBuilder struct {
	bw  *glfs.TypedWriter
	nlw *NodeListWriter
}

func NewNodeListBuilder(s cadata.Poster) NodeListBuilder {
	ag := glfs.NewAgent()
	bw := ag.NewBlobWriter(s)
	return NodeListBuilder{bw: bw, nlw: NewNodeListWriter(bw)}
}

func (b *NodeListBuilder) Add(x Node) (NodeID, error) {
	return b.nlw.Add(x)
}

func (b *NodeListBuilder) Finish(ctx context.Context) (*glfs.Ref, error) {
	return b.bw.Finish(ctx)
}

type NodeListWriter struct {
	enc       *json.Encoder
	nodeCount uint64
}

func NewNodeListWriter(w io.Writer) *NodeListWriter {
	return &NodeListWriter{enc: json.NewEncoder(w)}
}

func (gw *NodeListWriter) Add(x Node) (NodeID, error) {
	if x.Value != nil && x.Value.CID.IsZero() {
		return 0, errors.New("ref value cannot be zero")
	}
	nid := NodeID(gw.nodeCount)
	var inputs []nodeInput
	for _, input := range x.Inputs {
		if input.Node >= nid {
			return 0, fmt.Errorf("dangling reference")
		}
		inputs = append(inputs, nodeInput{
			To:   input.Name,
			From: NodeOffset(nid - input.Node),
		})
	}
	slices.SortFunc(inputs, func(a, b nodeInput) int {
		return strings.Compare(a.To, b.To)
	})
	if err := gw.enc.Encode(nodeODF{
		Value:  x.Value,
		OpName: x.Op,
		Inputs: inputs,
	}); err != nil {
		return 0, err
	}
	gw.nodeCount++
	return nid, nil
}

func (gw *NodeListWriter) Count() uint64 {
	return gw.nodeCount
}

type NodeListReader struct {
	dec      *json.Decoder
	nextNode NodeID
}

func NewNodeListReader(r io.Reader) *NodeListReader {
	return &NodeListReader{dec: json.NewDecoder(r)}
}

func (gr *NodeListReader) Next(ctx context.Context, dst *Node) error {
	if !gr.dec.More() {
		return streams.EOS()
	}
	var node nodeODF
	if err := gr.dec.Decode(&node); err != nil {
		return err
	}
	dst.Value = node.Value
	dst.Op = node.OpName
	dst.Inputs = dst.Inputs[:0]
	for _, input := range node.Inputs {
		if input.From == 0 {
			return fmt.Errorf("input %q has 0 offset", input.To)
		}
		dst.Inputs = append(dst.Inputs, NodeInput{
			Name: input.To,
			Node: gr.nextNode - NodeID(input.From),
		})
	}
	gr.nextNode++
	return nil
}

// NodeOffset is how nodes refer to other nodes as input in the wire format
type NodeOffset uint32

type nodeODF struct {
	Value  *glfs.Ref   `json:"v,omitempty"`
	OpName OpName      `json:"op,omitempty"`
	Inputs []nodeInput `json:"in,omitempty"`
}

type nodeInput struct {
	To   string
	From NodeOffset
}

func (ni nodeInput) MarshalJSON() (ret []byte, _ error) {
	p, _ := json.Marshal(ni.To)
	return fmt.Appendf(ret, `[%s, %d]`, p, ni.From), nil
}

func (ni *nodeInput) UnmarshalJSON(x []byte) error {
	var a [2]json.RawMessage
	if err := json.Unmarshal(x, &a); err != nil {
		return err
	}
	if err := json.Unmarshal(a[0], &ni.To); err != nil {
		return err
	}
	if err := json.Unmarshal(a[1], &ni.From); err != nil {
		return err
	}
	return nil
}
