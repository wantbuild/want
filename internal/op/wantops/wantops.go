// package wantops contains an executor for build system operations like compiling
package wantops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/glfstasks"
	"wantbuild.io/want/internal/stringsets"
	"wantbuild.io/want/internal/wantc"
	"wantbuild.io/want/internal/wantdag"
	"wantbuild.io/want/lib/wantcfg"
	"wantbuild.io/want/lib/wantjob"
)

const (
	OpCompile        = wantjob.OpName("compile")
	OpCompileSnippet = wantjob.OpName("compileSnippet")
	OpPathSetRegexp  = wantjob.OpName("pathSetRegexp")
)

const MaxSnippetSize = 1e7

var _ wantjob.Executor = &Executor{}

type Executor struct{}

func (e Executor) Execute(jc wantjob.Ctx, src cadata.Getter, x wantjob.Task) ([]byte, error) {
	ctx := jc.Context
	switch x.Op {
	case OpCompile:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			return e.Compile(ctx, jc.Dst, src, x)
		})
	case OpCompileSnippet:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			return e.CompileSnippet(ctx, jc.Dst, src, x)
		})
	case OpPathSetRegexp:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			return e.PathSetRegexp(jc, jc.Dst, src, x)
		})
	default:
		return nil, wantjob.NewErrUnknownOperator(x.Op)
	}
}

func (e Executor) Compile(ctx context.Context, dst cadata.Store, s cadata.Getter, x glfs.Ref) (*glfs.Ref, error) {
	ct, err := GetCompileTask(ctx, s, x)
	if err != nil {
		return nil, err
	}
	c := wantc.NewCompiler()
	plan, err := c.Compile(ctx, dst, s, ct.Metadata, ct.Ground)
	if err != nil {
		return nil, err
	}
	return wantc.PostPlan(ctx, dst, *plan)
}

func (e Executor) CompileSnippet(ctx context.Context, dst cadata.Store, s cadata.Getter, ref glfs.Ref) (*glfs.Ref, error) {
	c := wantc.NewCompiler()
	data, err := glfs.GetBlobBytes(ctx, s, ref, MaxSnippetSize)
	if err != nil {
		return nil, err
	}
	dag, err := c.CompileSnippet(ctx, dst, s, data)
	if err != nil {
		return nil, err
	}
	return wantdag.PostDAG(ctx, dst, *dag)
}

func (e Executor) PathSetRegexp(jc wantjob.Ctx, dst cadata.Store, s cadata.Getter, ref glfs.Ref) (*glfs.Ref, error) {
	ctx := jc.Context
	data, err := glfs.GetBlobBytes(ctx, s, ref, 1e6)
	if err != nil {
		return nil, err
	}
	var pathSet wantcfg.PathSet
	if err := json.Unmarshal(data, &pathSet); err != nil {
		return nil, err
	}
	set := wantc.SetFromQuery("", pathSet)
	stringsets.Simplify(set)
	jc.Infof("set: %v", set)
	re := set.Regexp()
	jc.Infof("re: %v", re)
	return glfs.PostBlob(ctx, dst, strings.NewReader(re.String()))
}

const MaxMetadataSize = 1 << 17

type CompileTask struct {
	Ground   glfs.Ref
	Metadata wantc.Metadata
}

func PostCompileTask(ctx context.Context, s cadata.Poster, x CompileTask) (*glfs.Ref, error) {
	mdJson, err := json.Marshal(x.Metadata)
	if err != nil {
		return nil, err
	}
	mdRef, err := glfs.PostBlob(ctx, s, bytes.NewReader(mdJson))
	if err != nil {
		return nil, err
	}
	return glfs.PostTreeMap(ctx, s, map[string]glfs.Ref{
		"ground":    x.Ground,
		"meta.json": *mdRef,
	})
}

func GetCompileTask(ctx context.Context, s cadata.Getter, x glfs.Ref) (*CompileTask, error) {
	if _, err := glfs.GetTree(ctx, s, x); err != nil {
		return nil, err
	}
	groundRef, err := glfs.GetAtPath(ctx, s, x, "ground")
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
		Ground:   *groundRef,
		Metadata: md,
	}, nil
}
