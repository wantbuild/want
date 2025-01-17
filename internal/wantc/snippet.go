package wantc

import (
	"context"
	"encoding/json"

	"github.com/google/go-jsonnet"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/wantdag"
	"wantbuild.io/want/lib/wantcfg"
)

// CompileSnippet turns a Jsonnet snippet into a wantdag.Graph.
// Values are loaded into the compiler's store.
// Selections are not allowed and will result in a compiler error.
func (c *Compiler) CompileSnippet(ctx context.Context, dst cadata.Store, src cadata.Getter, x []byte) (*wantdag.DAG, error) {
	vm := jsonnet.MakeVM()
	vm.Importer(libOnlyImporter{})
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
	dag := gb.Finish()
	return &dag, nil
}
