package want

import (
	"context"
	"runtime"

	"github.com/blobcache/glfs"
	"github.com/jmoiron/sqlx"
	"go.brendoncarroll.net/exp/slices2"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/glfstasks"
	"wantbuild.io/want/internal/op/dagops"
	"wantbuild.io/want/internal/stores"
	"wantbuild.io/want/internal/wantc"
	"wantbuild.io/want/internal/wantdag"
	"wantbuild.io/want/internal/wantjob"
	"wantbuild.io/want/lib/wantcfg"
	"wantbuild.io/want/lib/wantrepo"
)

// BuildResult is the output of a build
type BuildResult struct {
	// Prefix is the prefix within the main repo this build is for.
	Prefix        string
	OutputRoot    *glfs.Ref
	Targets       []Target
	TargetResults []TargetResult

	// TODO: remove
	Store cadata.Getter
}

// Target is a set of trees/files that will be generated as part of the build
type Target struct {
	// To is the pathset that this target controls
	To wantcfg.PathSet
	// From is the expr which defines the desired contents of the PathSet.
	From wantcfg.Expr
	// DefinedIn is where this target was defined.
	DefinedIn string
	// DefinedNum is the order the statement is defined in the file.
	DefinedNum int
}

type TargetResult struct {
	Root *glfs.Ref
}

func Build(ctx context.Context, db *sqlx.DB, repo *wantrepo.Repo, prefix string) (*BuildResult, error) {
	srcid, err := Import(ctx, db, repo)
	if err != nil {
		return nil, err
	}
	srcRoot, srcStore, err := AccessSource(ctx, db, srcid)
	if err != nil {
		return nil, err
	}

	cstore := stores.NewMem()
	c := wantc.NewCompiler(cstore)
	plan, err := c.Compile(ctx, srcStore, *srcRoot, prefix)
	if err != nil {
		return nil, err
	}

	exec := newExecutor(stores.NewMem())
	jsys := newJobSys(ctx, db, exec, runtime.GOMAXPROCS(0))
	defer jsys.Shutdown()

	dagRes, outStore, err := runRootJob(ctx, jsys, stores.Union{srcStore, cstore}, wantjob.Task{
		Op:    joinOpName("dag", dagops.OpExecAll),
		Input: glfstasks.MarshalGLFSRef(plan.Graph),
	})
	if err != nil {
		return nil, err
	}
	nrs, err := wantdag.GetNodeResults(ctx, outStore, *dagRes)
	if err != nil {
		return nil, err
	}
	rootRes := nrs[plan.Root]
	outRoot, err := glfstasks.ParseGLFSRef(rootRes.Data)
	if err != nil {
		return nil, err
	}
	return &BuildResult{
		Prefix:     prefix,
		OutputRoot: outRoot,
		Targets: slices2.Map(plan.Targets, func(x wantc.Target) Target {
			return Target{
				To:         x.To,
				DefinedIn:  x.DefinedIn,
				DefinedNum: 0, // TODO
			}
		}),
		Store: outStore,
	}, nil
}
