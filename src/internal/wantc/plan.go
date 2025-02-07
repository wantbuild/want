package wantc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/exp/streams"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/stringsets"
	"wantbuild.io/want/src/wantcfg"
)

// Target is a (Set[string], Expr) pair.
type Target struct {
	// To is where the Target outptus To.
	// The DAG will evaluate to a filesystem which only contains paths in this set.
	To wantcfg.PathSet `json:"to"`
	// DAG contains a compiled program to build the target
	DAG glfs.Ref `json:"dag"`
	// Expr is an expression which is equivalent to this target.
	// Expr will always be context-free. Meaning it does not depend on selections which
	// are relevant to the module it was compiled in.
	Expr wantcfg.Expr `json:"expr"`

	// DefinedIn is the path of the file that defines the Target
	DefinedIn string `json:"defined_in"`

	// IsStatement is true when the target is from a statement file
	IsStatement bool `json:"is_statement"`
	// If the Target is defined in a statement file, this will be the statement number
	DefinedNum int `json:"defined_num"`
	// IsExport is true when the file should be exported.
	IsExport bool `json:"is_export"`
}

func (t Target) BoundingPrefix() string {
	return stringsets.BoundingPrefix(SetFromQuery("", t.To))
}

// Plan is the result of compilation.
type Plan struct {
	Known   glfs.Ref `json:"known"`
	Targets []Target `json:"targets"`
}

func PostPlan(ctx context.Context, s cadata.PostExister, x Plan) (*glfs.Ref, error) {
	data, err := json.Marshal(x)
	if err != nil {
		return nil, err
	}
	planRef, err := glfs.PostBlob(ctx, s, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	ents := []glfs.TreeEntry{
		{Name: "plan.json", FileMode: 0o777, Ref: *planRef},
		{Name: "known", FileMode: 0o777, Ref: x.Known},
	}
	for i, target := range x.Targets {
		ents = append(ents, glfs.TreeEntry{
			Name:     fmt.Sprintf("dag_%08d", i),
			FileMode: 0o777,
			Ref:      target.DAG,
		})
	}
	return glfs.PostTreeSlice(ctx, s, ents)
}

func SyncPlan(ctx context.Context, dst cadata.PostExister, src cadata.Getter, plan Plan) error {
	allRefs := func(yield func(glfs.Ref) bool) {
		for _, target := range plan.Targets {
			if !yield(target.DAG) {
				return
			}
		}
		if !yield(plan.Known) {
			return
		}
	}
	for ref := range allRefs {
		if err := glfs.Sync(ctx, dst, src, ref); err != nil {
			return err
		}
	}
	return nil
}

const MaxPlanSize = 1e6

func GetPlan(ctx context.Context, s cadata.Getter, x glfs.Ref) (*Plan, error) {
	ag := glfs.NewAgent()
	tr, err := ag.NewTreeReader(s, x)
	if err != nil {
		return nil, err
	}
	var plan Plan
	var dags []glfs.Ref
	for {
		ent, err := streams.Next(ctx, tr)
		if err != nil {
			if streams.IsEOS(err) {
				break
			}
			return nil, err
		}
		switch {
		case ent.Name == "plan.json":
			data, err := ag.GetBlobBytes(ctx, s, ent.Ref, MaxPlanSize)
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal(data, &plan); err != nil {
				return nil, err
			}
		case ent.Name == "known":
			plan.Known = ent.Ref
		case strings.HasPrefix(ent.Name, "dag_"):
			dags = append(dags, ent.Ref)
		default:
			return nil, fmt.Errorf("plan tree has unknown entry %s", ent.Name)
		}
	}
	if plan.Known == (glfs.Ref{}) {
		return nil, fmt.Errorf("plan is missing known tree")
	}
	if len(plan.Targets) != len(dags) {
		return nil, fmt.Errorf("plan has the wrong number of DAGs")
	}
	for i := range dags {
		if !dags[i].Equals(plan.Targets[i].DAG) {
			return nil, fmt.Errorf("mismatched DAG refs at %d", i)
		}
	}
	return &plan, nil
}
