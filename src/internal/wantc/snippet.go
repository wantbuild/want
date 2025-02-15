package wantc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/go-jsonnet"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/wantdag"
	"wantbuild.io/want/src/wantcfg"
)

// CompileSnippet turns a Jsonnet snippet into a wantdag.Graph.
// Values are loaded into the compiler's store.
// Selections are not allowed and will result in a compiler error.
func (c *Compiler) CompileSnippet(ctx context.Context, dst cadata.Store, src cadata.Getter, x []byte) (wantdag.DAG, error) {
	vm := jsonnet.MakeVM()
	vm.Importer(&snippetImporter{})
	jsonData, err := vm.EvaluateAnonymousSnippet("", string(x))
	if err != nil {
		return nil, err
	}
	var expr wantcfg.Expr
	if err := json.Unmarshal([]byte(jsonData), &expr); err != nil {
		return nil, err
	}
	return c.CompileExpr(ctx, dst, src, expr)
}

// CompileExpr takes a Expr in the build language and produces a DAG that will evaluate it.
// The Expr is evaluated in the snippet context.
func (c *Compiler) CompileExpr(ctx context.Context, dst cadata.Store, src cadata.Getter, x wantcfg.Expr) (wantdag.DAG, error) {
	vm := jsonnet.MakeVM()
	vm.Importer(&snippetImporter{})

	cc := &compileCtx{ctx: ctx, dst: dst, src: src}
	expr, err := c.compileExpr(cc, "", x)
	if err != nil {
		return nil, err
	}
	gb := NewGraphBuilder(dst)
	if _, err := gb.Expr(ctx, src, expr); err != nil {
		return nil, err
	}
	return gb.Finish(), nil
}

type snippetImporter struct {
	libWant jsonnet.Contents
}

func (imp *snippetImporter) Import(importedFrom, importPath string) (jsonnet.Contents, string, error) {
	if importPath == "@want" {
		if imp.libWant == (jsonnet.Contents{}) {
			imp.libWant = jsonnet.MakeContents(LibWant())
		}
		return imp.libWant, "@want", nil
	}
	return jsonnet.Contents{}, "", fmt.Errorf("imports other than @want are not allowed in snippet")
}
