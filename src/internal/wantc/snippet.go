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
	vm.Importer(snippetImporter{})
	jsonData, err := vm.EvaluateSnippet("", string(x))
	if err != nil {
		return nil, err
	}
	var spec wantcfg.Expr
	if err := json.Unmarshal([]byte(jsonData), &spec); err != nil {
		return nil, err
	}
	cs := &compileState{dst: dst, src: src}
	expr, err := c.compileExpr(ctx, cs, "", spec)
	if err != nil {
		return nil, err
	}
	gb := NewGraphBuilder(dst)
	if _, err := gb.Expr(ctx, src, expr); err != nil {
		return nil, err
	}
	return gb.Finish(), nil
}

type snippetImporter struct{}

func (snippetImporter) Import(importedFrom, importPath string) (jsonnet.Contents, string, error) {
	if importPath == "want" {
		return jsonnet.MakeContents(wantcfg.LibWant()), "want", nil
	}
	return jsonnet.Contents{}, "", fmt.Errorf("imports not allowed in snippet")
}
