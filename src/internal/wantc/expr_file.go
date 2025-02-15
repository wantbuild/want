package wantc

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"wantbuild.io/want/src/internal/stringsets"
	"wantbuild.io/want/src/wantcfg"
)

// ExprRoot represents a Want expression tree to be evaluated in the build.
// It will produce >= 1 node in the computation graph.
type exprRoot struct {
	spec wantcfg.Expr
	path string

	expr Expr
}

// newExpr creates an *ExprRoot from a spec.
// If the spec specifies any literals they will be posted to the store.
func (c *Compiler) parseExprRoot(cc *compileCtx, fqp FQPath) (*exprRoot, error) {
	vm := newJsonnetVM(cc.jsImporter, cc.buildCtx)
	fqp.Path = strings.Trim(fqp.Path, "/")
	jsonStr, err := vm.EvaluateFile(mkJsonnetPath(fqp))
	if err != nil {
		return nil, err
	}
	var spec wantcfg.Expr
	if err := json.Unmarshal([]byte(jsonStr), &spec); err != nil {
		return nil, fmt.Errorf("error in stage file %q: %w", fqp.Path, err)
	}
	e, err := c.compileExpr(cc, fqp.Path, spec)
	if err != nil {
		return nil, err
	}
	return &exprRoot{
		spec: spec,
		path: fqp.Path,

		expr: e,
	}, nil
}

func (s *exprRoot) Affects() stringsets.Set {
	return stringsets.Union(stringsets.Unit(s.path), stringsets.Prefix(s.path+"/"))
}

func (s *exprRoot) Needs() stringsets.Set {
	return s.expr.Needs()
}

func (s *exprRoot) String() string {
	return fmt.Sprintf("Expr{%s}", s.path)
}

func (s *exprRoot) Pretty(w io.Writer) string {
	sb := strings.Builder{}
	if err := s.PrettyPrint(&sb); err != nil {
		panic(err)
	}
	return sb.String()
}

func (s *exprRoot) PrettyPrint(w io.Writer) error {
	return s.expr.PrettyPrint(w)
}

func (s *exprRoot) Path() string {
	return s.path
}
