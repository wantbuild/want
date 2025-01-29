// package wantops contains an executor for build system operations like compiling
package wantops

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/glfstasks"
	"wantbuild.io/want/internal/stores"
	"wantbuild.io/want/internal/stringsets"
	"wantbuild.io/want/internal/wantc"
	"wantbuild.io/want/internal/wantdag"
	"wantbuild.io/want/lib/wantcfg"
	"wantbuild.io/want/lib/wantjob"
)

const (
	OpBuild          = wantjob.OpName("build")
	OpCompile        = wantjob.OpName("compile")
	OpCompileSnippet = wantjob.OpName("compileSnippet")
	OpPathSetRegexp  = wantjob.OpName("pathSetRegexp")
)

const MaxSnippetSize = 1e7

var _ wantjob.Executor = &Executor{}

type Executor struct {
	CompileOp wantjob.OpName
	DAGExecOp wantjob.OpName
}

func (e Executor) Execute(jc wantjob.Ctx, src cadata.Getter, x wantjob.Task) ([]byte, error) {
	ctx := jc.Context
	switch x.Op {
	case OpBuild:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			return e.Build(jc, src, x)
		})
	case OpCompile:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			return e.Compile(jc, src, x)
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

func (e Executor) Build(jc wantjob.Ctx, src cadata.Getter, x glfs.Ref) (*glfs.Ref, error) {
	defer jc.InfoSpan("build")()
	ctx := jc.Context
	buildTask, err := GetBuildTask(ctx, src, x)
	if err != nil {
		return nil, err
	}
	// plan
	plan, planStore, err := DoCompile(ctx, jc.System, e.CompileOp, src, CompileTask{
		Module:   buildTask.Main,
		Metadata: buildTask.Metadata,
	})
	if err != nil {
		return nil, err
	}
	if buildTask.Prefix != "" {
		dagRef, err := wantdag.EditDAG(ctx, jc.Dst, planStore, plan.DAG, func(x wantdag.DAG) (*wantdag.DAG, error) {
			last := wantdag.NodeID(len(x.Nodes) - 1)
			// path
			pathRef, err := glfs.PostBlob(ctx, jc.Dst, strings.NewReader(buildTask.Prefix))
			if err != nil {
				return nil, err
			}
			x.Nodes = append(x.Nodes, wantdag.Node{
				Value: pathRef,
			})
			// derive pick
			x.Nodes = append(x.Nodes, wantdag.Node{
				Op: "glfs.pick",
				Inputs: []wantdag.NodeInput{
					{Name: "x", Node: last},
					{Name: "path", Node: last + 1},
				},
			})
			return &x, nil
		})

		if err != nil {
			return nil, err
		}
		plan.LastNode += 2 // Change this if you add more nodes above
		plan.DAG = *dagRef
	}
	// execute
	s := stores.Union{src, planStore, jc.Dst}
	dagResRef, dagStore, err := glfstasks.Do(jc.Context, jc.System, s, e.DAGExecOp, plan.DAG)
	if err != nil {
		return nil, err
	}
	df := jc.InfoSpan("syncing results")
	if err := glfstasks.FastSync(ctx, jc.Dst, dagStore, *dagResRef); err != nil {
		return nil, err
	}
	df()
	nrs, err := wantdag.GetNodeResults(ctx, dagStore, *dagResRef)
	if err != nil {
		return nil, err
	}
	return PostBuildResult(ctx, jc.Dst, BuildResult{
		Plan:        *plan,
		NodeResults: nrs,
	})
}

func (e Executor) Compile(jc wantjob.Ctx, s cadata.Getter, x glfs.Ref) (*glfs.Ref, error) {
	defer jc.InfoSpan("compile")()
	ctx := jc.Context
	dst := jc.Dst
	ct, err := GetCompileTask(ctx, s, x)
	if err != nil {
		return nil, err
	}
	c := wantc.NewCompiler()
	plan, err := c.Compile(ctx, dst, s, ct.Metadata, ct.Module)
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

func DoCompile(ctx context.Context, sys wantjob.System, compileOp wantjob.OpName, src cadata.Getter, ct CompileTask) (*wantc.Plan, cadata.Getter, error) {
	scratch := stores.NewMem()
	ctRef, err := PostCompileTask(ctx, scratch, ct)
	if err != nil {
		return nil, nil, err
	}
	planRef, planStore, err := glfstasks.Do(ctx, sys, stores.Union{src, scratch}, compileOp, *ctRef)
	if err != nil {
		return nil, nil, err
	}
	plan, err := wantc.GetPlan(ctx, planStore, *planRef)
	if err != nil {
		return nil, nil, err
	}
	return plan, planStore, nil
}
