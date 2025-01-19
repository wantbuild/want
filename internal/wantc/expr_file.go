package wantc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"wantbuild.io/want/internal/stringsets"
	"wantbuild.io/want/lib/wantcfg"
)

// ExprRoot represents a Want expression tree to be evaluated in the build.
// It will produce >= 1 node in the computation graph.
type ExprRoot struct {
	spec wantcfg.Expr
	path string

	expr Expr
}

// newExpr creates an *ExprRoot from a spec.
// If the spec specifies any literals they will be posted to the store.
func (c *Compiler) parseExprRoot(ctx context.Context, cs *compileState, p string, data []byte) (*ExprRoot, error) {
	vm := newJsonnetVM(cs.jsImporter, cs.buildCtx)
	p = strings.Trim(p, "/")
	jsonStr, err := vm.EvaluateSnippet(p, string(data))
	if err != nil {
		return nil, err
	}
	var spec wantcfg.Expr
	if err := json.Unmarshal([]byte(jsonStr), &spec); err != nil {
		return nil, fmt.Errorf("error in stage file %q: %w", p, err)
	}
	e, err := c.compileExpr(ctx, cs, p, spec)
	if err != nil {
		return nil, err
	}
	return &ExprRoot{
		spec: spec,
		path: p,

		expr: e,
	}, nil
}

func (s *ExprRoot) Affects() stringsets.Set {
	return stringsets.Union(stringsets.Single(s.path), stringsets.Prefix(s.path+"/"))
}

func (s *ExprRoot) Needs() stringsets.Set {
	return s.expr.Needs()
}

func (s *ExprRoot) String() string {
	return fmt.Sprintf("Expr{%s}", s.path)
}

func (s *ExprRoot) Pretty(w io.Writer) string {
	sb := strings.Builder{}
	if err := s.PrettyPrint(&sb); err != nil {
		panic(err)
	}
	return sb.String()
}

func (s *ExprRoot) PrettyPrint(w io.Writer) error {
	return s.expr.PrettyPrint(w)
}

func (s *ExprRoot) Path() string {
	return s.path
}
