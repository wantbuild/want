// package wantops contains an executor for build system operations like compiling
package wantops

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/stringsets"
	"wantbuild.io/want/internal/wantc"
	"wantbuild.io/want/internal/wantdag"
	"wantbuild.io/want/internal/wantjob"
	"wantbuild.io/want/lib/wantcfg"
)

const (
	OpCompile        = wantjob.OpName("compile")
	OpCompileSnippet = wantjob.OpName("compileSnippet")
	OpPathSetRegexp  = wantjob.OpName("pathSetRegexp")
)

const MaxSnippetSize = 1e7

var _ wantjob.Executor = &Executor{}

type Executor struct {
	s    cadata.Store
	glfs *glfs.Agent
	c    *wantc.Compiler
}

func NewExecutor(s cadata.Store) Executor {
	return Executor{
		s:    s,
		glfs: glfs.NewAgent(),
		c:    wantc.NewCompiler(s),
	}
}

func (e Executor) Compute(ctx context.Context, jc *wantjob.JobCtx, src cadata.Getter, x wantjob.Task) (*glfs.Ref, error) {
	switch x.Op {
	case OpCompile:
		return e.Compile(ctx, src, x.Input)
	case OpCompileSnippet:
		return e.CompileSnippet(ctx, src, x.Input)
	case OpPathSetRegexp:
		return e.PathSetRegexp(ctx, jc, src, x.Input)
	default:
		return nil, wantjob.NewErrUnknownOperator(x.Op)
	}
}

func (e Executor) GetStore() cadata.Getter {
	return e.s
}

func (e Executor) Compile(ctx context.Context, s cadata.Getter, ref glfs.Ref) (*glfs.Ref, error) {
	plan, err := e.c.Compile(ctx, ref, "")
	if err != nil {
		return nil, err
	}
	return &plan.Graph, nil
}

func (e Executor) CompileSnippet(ctx context.Context, s cadata.Getter, ref glfs.Ref) (*glfs.Ref, error) {
	data, err := e.glfs.GetBlobBytes(ctx, s, ref, MaxSnippetSize)
	if err != nil {
		return nil, err
	}
	dag, err := e.c.CompileSnippet(ctx, data)
	if err != nil {
		return nil, err
	}
	return wantdag.PostDAG(ctx, e.s, *dag)
}

func (e Executor) PathSetRegexp(ctx context.Context, jc *wantjob.JobCtx, s cadata.Getter, ref glfs.Ref) (*glfs.Ref, error) {
	data, err := e.glfs.GetBlobBytes(ctx, s, ref, 1e6)
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
	return e.glfs.PostBlob(ctx, e.s, strings.NewReader(re.String()))
}
