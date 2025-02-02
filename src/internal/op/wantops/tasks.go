package wantops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"
	"wantbuild.io/want/src/internal/wantc"
	"wantbuild.io/want/src/internal/wantdag"
	"wantbuild.io/want/src/wantcfg"
	"wantbuild.io/want/src/wantjob"
)

const MaxMetadataSize = 1 << 17

// BuildConfig is the contents of build.json
type BuildConfig struct {
	Query    wantcfg.PathSet `json:"query"`
	Metadata wantc.Metadata  `json:"metadata"`
}

// BuildTask can be encoded as a GLFS Tree.
type BuildTask struct {
	Main glfs.Ref

	Query    wantcfg.PathSet
	Metadata wantc.Metadata
}

func PostBuildTask(ctx context.Context, s cadata.PostExister, x BuildTask) (*glfs.Ref, error) {
	cfgJson, err := json.Marshal(BuildConfig{
		Query:    x.Query,
		Metadata: x.Metadata,
	})
	if err != nil {
		return nil, err
	}
	cfgRef, err := glfs.PostBlob(ctx, s, bytes.NewReader(cfgJson))
	if err != nil {
		return nil, err
	}
	return glfs.PostTreeMap(ctx, s, map[string]glfs.Ref{
		"main":       x.Main,
		"build.json": *cfgRef,
	})
}

func GetBuildTask(ctx context.Context, s cadata.Getter, x glfs.Ref) (*BuildTask, error) {
	mainRef, err := glfs.GetAtPath(ctx, s, x, "main")
	if err != nil {
		return nil, err
	}
	cfgRef, err := glfs.GetAtPath(ctx, s, x, "build.json")
	if err != nil {
		return nil, err
	}
	cfgJson, err := glfs.GetBlobBytes(ctx, s, *cfgRef, MaxMetadataSize)
	if err != nil {
		return nil, err
	}
	var cfg BuildConfig
	if err := json.Unmarshal(cfgJson, &cfg); err != nil {
		return nil, err
	}
	return &BuildTask{
		Main:     *mainRef,
		Query:    cfg.Query,
		Metadata: cfg.Metadata,
	}, nil
}

type BuildResult struct {
	// Query is the PathSet to build targets from
	Query wantcfg.PathSet `json:"query"`
	Plan  wantc.Plan      `json:"plan"`
	// Targets is the subset of targets that were run
	Targets       []wantc.Target   `json:"targets"`
	TargetResults []wantjob.Result `json:"target_results"`
	Output        *glfs.Ref        `json:"output"`
}

func PostBuildResult(ctx context.Context, s cadata.PostExister, x BuildResult) (*glfs.Ref, error) {
	planRef, err := wantc.PostPlan(ctx, s, x.Plan)
	if err != nil {
		return nil, err
	}
	nrRef, err := wantdag.PostNodeResults(ctx, s, x.TargetResults)
	if err != nil {
		return nil, err
	}
	cfgJson, err := json.Marshal(x)
	if err != nil {
		return nil, err
	}
	cfgRef, err := glfs.PostBlob(ctx, s, bytes.NewReader(cfgJson))
	if err != nil {
		return nil, err
	}
	ents := map[string]glfs.Ref{
		"plan":               *planRef,
		"target_results":     *nrRef,
		"build_results.json": *cfgRef,
	}
	if x.Output != nil {
		ents["output"] = *x.Output
	}
	return glfs.PostTreeMap(ctx, s, ents)
}

func GetBuildResult(ctx context.Context, s cadata.Getter, x glfs.Ref) (*BuildResult, error) {
	// config
	cfgRef, err := glfs.GetAtPath(ctx, s, x, "build_results.json")
	if err != nil {
		return nil, err
	}
	cfgJson, err := glfs.GetBlobBytes(ctx, s, *cfgRef, 1e6)
	if err != nil {
		return nil, err
	}
	var ret BuildResult
	if err := json.Unmarshal(cfgJson, &ret); err != nil {
		return nil, err
	}
	// plan
	planRef, err := glfs.GetAtPath(ctx, s, x, "plan")
	if err != nil {
		return nil, err
	}
	plan, err := wantc.GetPlan(ctx, s, *planRef)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(*plan, ret.Plan) {
		return nil, fmt.Errorf("invalid build result, plan mismatch")
	}
	// target results
	nrRef, err := glfs.GetAtPath(ctx, s, x, "target_results")
	if err != nil {
		return nil, err
	}
	results, err := wantdag.GetNodeResults(ctx, s, *nrRef)
	if err != nil {
		return nil, err
	}
	if len(results) != len(ret.TargetResults) {
		return nil, fmt.Errorf("invalid build result, target results mismatch %v %v", len(results), len(ret.TargetResults))
	}
	outRef, err := glfs.GetAtPath(ctx, s, x, "output")
	if err != nil && !glfs.IsErrNoEnt(err) {
		return nil, err
	}
	ret.Output = outRef
	// TODO: validate
	return &ret, nil
}

type CompileTask struct {
	Module   glfs.Ref
	Metadata wantc.Metadata
}

func PostCompileTask(ctx context.Context, s cadata.PostExister, x CompileTask) (*glfs.Ref, error) {
	mdJson, err := json.Marshal(x.Metadata)
	if err != nil {
		return nil, err
	}
	mdRef, err := glfs.PostBlob(ctx, s, bytes.NewReader(mdJson))
	if err != nil {
		return nil, err
	}
	return glfs.PostTreeMap(ctx, s, map[string]glfs.Ref{
		"module":    x.Module,
		"meta.json": *mdRef,
	})
}

func GetCompileTask(ctx context.Context, s cadata.Getter, x glfs.Ref) (*CompileTask, error) {
	moduleRef, err := glfs.GetAtPath(ctx, s, x, "module")
	if err != nil {
		return nil, err
	}
	metaRef, err := glfs.GetAtPath(ctx, s, x, "meta.json")
	if err != nil {
		return nil, err
	}
	data, err := glfs.GetBlobBytes(ctx, s, *metaRef, MaxMetadataSize)
	if err != nil {
		return nil, err
	}
	var md wantc.Metadata
	if err := json.Unmarshal(data, &md); err != nil {
		return nil, fmt.Errorf("meta.json did not contain valid json: %q, %w", data, err)
	}
	return &CompileTask{
		Module:   *moduleRef,
		Metadata: md,
	}, nil
}
