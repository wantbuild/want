package wantc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/stringsets"
	"wantbuild.io/want/src/internal/wantdag"
	"wantbuild.io/want/src/wantcfg"
)

// Target is a (Set[string], Expr) pair.
type Target struct {
	To wantcfg.PathSet `json:"to"`
	// Node is the node in the graph which can be evaluated to produce this target
	Node wantdag.NodeID `json:"node"`
	// Expr is an expression which is equivalent to this target
	Expr wantcfg.Expr `json:"expr"`

	// IsDerived is true when this target is computed
	IsDerived bool `json:"is_derived"`
	// IsStatement is true when the target is from a statement file
	IsStatement bool `json:"is_statement"`
	// IsExport is true when the file should be exported.
	IsExport bool `json:"is_export"`
	// DefinedIn is the path of the file that defines the Target
	DefinedIn string `json:"defined_in"`
	// If the Target is defined in a statement file, this will be the statement number
	DefinedNum int `json:"defined_num"`
}

func (t Target) BoundingPrefix() string {
	return stringsets.BoundingPrefix(SetFromQuery("", t.To))
}

// Plan is the result of compilation.
type Plan struct {
	// DAG is the graph which can be executed to derive all targets
	DAG      glfs.Ref       `json:"graph"`
	LastNode wantdag.NodeID `json:"last_node"`
	Targets  []Target       `json:"targets"`
}

func (p *Plan) GetDAG(ctx context.Context, s cadata.Getter) (*wantdag.DAG, error) {
	return wantdag.GetDAG(ctx, s, p.DAG)
}

func PostPlan(ctx context.Context, s cadata.Poster, x Plan) (*glfs.Ref, error) {
	data, err := json.Marshal(x)
	if err != nil {
		return nil, err
	}
	planRef, err := glfs.PostBlob(ctx, s, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return glfs.PostTreeMap(ctx, s, map[string]glfs.Ref{
		"dag":       x.DAG,
		"plan.json": *planRef,
	})
}

func GetPlan(ctx context.Context, s cadata.Getter, x glfs.Ref) (*Plan, error) {
	dagRef, err := glfs.GetAtPath(ctx, s, x, "dag")
	if err != nil {
		return nil, err
	}
	planRef, err := glfs.GetAtPath(ctx, s, x, "plan.json")
	if err != nil {
		return nil, err
	}
	planData, err := glfs.GetBlobBytes(ctx, s, *planRef, 1<<20)
	if err != nil {
		return nil, err
	}
	var plan Plan
	if err := json.Unmarshal(planData, &plan); err != nil {
		return nil, err
	}
	if !dagRef.Equals(plan.DAG) {
		return nil, fmt.Errorf("DAG Ref in plan.json does not match actual")
	}
	return &plan, nil
}
