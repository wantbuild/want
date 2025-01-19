package wantc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"

	"wantbuild.io/want/internal/stringsets"
	"wantbuild.io/want/lib/wantcfg"
)

type StmtSet struct {
	path  string
	specs []wantcfg.Statement

	stmts []Stmt
}

func (c *Compiler) parseStmtSet(ctx context.Context, cs *compileState, p string, data []byte) (*StmtSet, error) {
	vm := newJsonnetVM(cs.jsImporter, cs.buildCtx)
	jsonStr, err := vm.EvaluateSnippet(p, string(data))
	if err != nil {
		return nil, err
	}
	var specs []wantcfg.Statement
	if err := json.Unmarshal([]byte(jsonStr), &specs); err != nil {
		return nil, fmt.Errorf("error in stage file %s: %w", p, err)
	}

	var stmts []Stmt
	for _, spec := range specs {
		var stmt Stmt
		switch {
		case spec.Put != nil:
			ks := SetFromQuery(p, spec.Put.Dst)
			e, err := c.compileExpr(ctx, cs, p, spec.Put.Src)
			if err != nil {
				return nil, err
			}
			stmt = &putStmt{
				Dst: ks,
				Src: e,
			}
		case spec.Export != nil:
			ks := SetFromQuery(p, spec.Export.Dst)
			e, err := c.compileExpr(ctx, cs, p, spec.Export.Src)
			if err != nil {
				return nil, err
			}
			stmt = &exportStmt{
				Dst: ks,
				Src: e,
			}
		default:
			return nil, errors.New("empty statement")
		}
		stmts = append(stmts, stmt)
	}
	return &StmtSet{
		path:  p,
		specs: specs,
		stmts: stmts,
	}, nil
}

func (sl *StmtSet) Needs() stringsets.Set {
	var ss []stringsets.Set
	for _, spec := range sl.stmts {
		ss = append(ss, spec.Needs())
	}
	return stringsets.Union(ss...)
}

func (sl *StmtSet) Affects() stringsets.Set {
	var ss []stringsets.Set
	for _, spec := range sl.stmts {
		ss = append(ss, spec.Affects())
	}
	return stringsets.Union(ss...)
}

type Stmt interface {
	Affects() stringsets.Set
	Needs() stringsets.Set
	expr() Expr
	setExpr(Expr)

	isStmt()
}

type putStmt struct {
	Dst stringsets.Set
	Src Expr
}

func (*putStmt) isStmt() {}

func (s *putStmt) Affects() stringsets.Set {
	return s.Dst
}

func (s *putStmt) Needs() stringsets.Set {
	return s.Src.Needs()
}

func (s *putStmt) expr() Expr {
	return s.Src
}

func (s *putStmt) setExpr(x Expr) {
	s.Src = x
}

type exportStmt struct {
	Dst stringsets.Set
	Src Expr
}

func (*exportStmt) isStmt() {}

func (s *exportStmt) Affects() stringsets.Set {
	return s.Dst
}

func (s *exportStmt) Needs() stringsets.Set {
	return s.Src.Needs()
}

func (s *exportStmt) expr() Expr {
	return s.Src
}

func (s *exportStmt) setExpr(x Expr) {
	s.Src = x
}
