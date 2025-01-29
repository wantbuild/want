package wantsetup

import (
	"context"
	"path/filepath"

	"wantbuild.io/want/internal/glfsport"
	"wantbuild.io/want/internal/glfstasks"
	"wantbuild.io/want/internal/op/glfsops"
	"wantbuild.io/want/internal/op/importops"
	"wantbuild.io/want/internal/stores"
	"wantbuild.io/want/internal/wantc"
	"wantbuild.io/want/internal/wantdag"
	"wantbuild.io/want/lib/wantjob"
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
	jc := wantjob.Ctx{Context: ctx, Dst: s, System: jsys}
	res, err := wantdag.SerialExecLast(jc, s, *dag)
	if err != nil {
		return err
	}
	if err := res.Err(); err != nil {
		return err
	}
	ref, err := glfstasks.ParseGLFSRef(res.Data)
	if err != nil {
		return err
	}
	exp := glfsport.Exporter{
		Dir:   outDir,
		Store: s,
		Cache: glfsport.NullCache{},
	}
	return exp.Export(ctx, *ref, "")
}

func NewExecutor() wantjob.MultiExecutor {
	return wantjob.MultiExecutor{
		"import": importops.NewExecutor(),
		"glfs":   glfsops.Executor{},
	}
}
