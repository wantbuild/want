package wantsetup

import (
	"context"
	"path/filepath"

	"wantbuild.io/want/src/internal/glfsport"
	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/internal/op/dagops"
	"wantbuild.io/want/src/internal/op/glfsops"
	"wantbuild.io/want/src/internal/op/importops"
	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/internal/wantc"
	"wantbuild.io/want/src/internal/wantdag"
	"wantbuild.io/want/src/wantjob"
)

func Install(ctx context.Context, jsys wantjob.System, outDir string, snippet string) error {
	outDir, err := filepath.Abs(outDir)
	if err != nil {
		return err
	}
	s := stores.NewMem()
	c := wantc.NewCompiler()
	dag, err := c.CompileSnippet(ctx, s, stores.NewVoid(), []byte(snippet))
	if err != nil {
		return err
	}
	dagRef, err := wantdag.PostDAG(ctx, s, dag)
	if err != nil {
		return err
	}
	res, outStore, err := wantjob.Do(ctx, jsys, s, wantjob.Task{
		Op:    "dag.execLast",
		Input: glfstasks.MarshalGLFSRef(*dagRef),
	})
	if err != nil {
		return err
	}
	if err := res.Err(); err != nil {
		return err
	}
	ref, err := glfstasks.ParseGLFSRef(res.Root)
	if err != nil {
		return err
	}
	exp := glfsport.Exporter{
		Dir:   outDir,
		Store: outStore,
		Cache: glfsport.NullCache{},
	}
	return exp.Export(ctx, *ref, "")
}

func NewExecutor() wantjob.MultiExecutor {
	return wantjob.MultiExecutor{
		"dag":    dagops.Executor{},
		"import": importops.NewExecutor(),
		"glfs":   glfsops.Executor{},
	}
}
