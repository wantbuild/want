package wantc

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"

	"wantbuild.io/want/src/internal/stringsets"
	"wantbuild.io/want/src/wantcfg"
)

type stmtSet struct {
	path  string
	specs []wantcfg.Statement

	stmts []*putStmt
}

func (c *Compiler) parseStmtSet(cc *compileCtx, fqp FQPath) (*stmtSet, error) {
	vm := newJsonnetVM(cc.jsImporter, cc.buildCtx)
	jsonStr, err := vm.EvaluateFile(mkJsonnetPath(fqp))
	if err != nil {
		return nil, err
	}
	var specs []wantcfg.Statement
	if err := json.Unmarshal([]byte(jsonStr), &specs); err != nil {
		return nil, fmt.Errorf("error in stage file %s: %w", fqp.Path, err)
	}

	var stmts []*putStmt
	for _, spec := range specs {
		var stmt *putStmt
		switch {
		case spec.Put != nil:
			ks := SetFromQuery(fqp.Path, spec.Put.Dst)
			e, err := c.compileExpr(cc, fqp.Path, spec.Put.Src)
			if err != nil {
				return nil, err
			}
			stmt = &putStmt{
				Dst: ks,
				Src: e,
			}
		default:
			return nil, errors.New("empty statement")
		}
		stmts = append(stmts, stmt)
	}
	return &stmtSet{
		path:  fqp.Path,
		specs: specs,
		stmts: stmts,
	}, nil
}

func (sl *stmtSet) Needs() stringsets.Set {
	var ss []stringsets.Set
	for _, spec := range sl.stmts {
		ss = append(ss, spec.Needs())
	}
	return stringsets.Union(ss...)
}

func (sl *stmtSet) Affects() stringsets.Set {
	var ss []stringsets.Set
	for _, spec := range sl.stmts {
		ss = append(ss, spec.Affects())
	}
	return stringsets.Union(ss...)
}

type putStmt struct {
	Dst stringsets.Set
	Src Expr
}

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
