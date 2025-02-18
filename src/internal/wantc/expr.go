package wantc

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"strconv"
	"strings"

	"github.com/blobcache/glfs"
	"github.com/kr/text"
	"github.com/pkg/errors"
	"go.brendoncarroll.net/state/cadata"
	"lukechampine.com/blake3"

	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/internal/stringsets"
	"wantbuild.io/want/src/internal/wantdag"
	"wantbuild.io/want/src/wantcfg"
)

// Expr is a node in an expression tree.
// Expressions are evaluated into values at build time.
type Expr interface {
	PrettyPrint(w io.Writer) error
	String() string
	Key() [32]byte
	Needs() stringsets.Set

	isExpr()
}

// computeInput is an input to compute
type computeInput struct {
	To   string
	From Expr
	Mode fs.FileMode
}

// compute is a compute expr, a variant of expr
type compute struct {
	Op     wantdag.OpName
	Inputs []computeInput
}

func (c *compute) Key() (ret [32]byte) {
	var x []byte
	// op
	x = binary.BigEndian.AppendUint32(x, uint32(len(c.Op)))
	x = append(x, []byte(c.Op)...)
	// inputs
	x = binary.BigEndian.AppendUint32(x, uint32(len(c.Inputs)))
	for _, in := range c.Inputs {
		x = binary.BigEndian.AppendUint32(x, uint32(len(in.To)))
		x = append(x, []byte(in.To)...)
		k := in.From.Key()
		x = append(x, k[:]...)
	}
	return blake3.Sum256(x)
}

func (c *compute) PrettyPrint(w io.Writer) error {
	if len(c.Inputs) == 0 {
		fmt.Fprintf(w, "(%s)", c.Op)
	} else if len(c.Inputs) == 1 && c.Inputs[0].To == "" {
		in := c.Inputs[0]
		fmt.Fprintf(w, "(%s ", c.Op)
		if err := in.From.PrettyPrint(w); err != nil {
			return err
		}
		fmt.Fprintf(w, ")")
	} else {
		fmt.Fprintf(w, "%s(\n", c.Op)
		w2 := text.NewIndentWriter(w, []byte("    "))
		l := maxToLen(c.Inputs)
		for _, in := range c.Inputs {
			to := in.To
			fmt.Fprintf(w2, "%-"+strconv.Itoa(l+2)+"q = ", to)
			if err := in.From.PrettyPrint(w2); err != nil {
				return err
			}
			fmt.Fprintf(w2, "\n")
		}
		fmt.Fprintf(w, ")")
	}
	return nil
}

func (c *compute) Needs() (ret stringsets.Set) {
	var sets []stringsets.Set
	for _, input := range c.Inputs {
		sets = append(sets, input.From.Needs())
	}
	return stringsets.Union(sets...)
}

func (c *compute) String() string {
	return fmt.Sprintf("(%v %v)", c.Op, c.Inputs)
}

func (c compute) isExpr() {}

type selection struct {
	module  ModuleID
	derived bool
	set     stringsets.Set
}

func newSelection(modID ModuleID, derived bool, set stringsets.Set) *selection {
	return &selection{
		module:  modID,
		derived: derived,
		set:     set,
	}
}

func (s *selection) PrettyPrint(w io.Writer) error {
	_, err := fmt.Fprintf(w, "select %v", s)
	return err
}

func (s *selection) Key() [32]byte {
	var x []byte
	x = append(x, []byte(s.set.String())...)
	return blake3.Sum256(x)
}

func (f *selection) Needs() stringsets.Set {
	return f.set
}

func (s *selection) String() string {
	return fmt.Sprintf("(select %v)", stringsets.ToPathSet(s.set))
}

func (s *selection) isExpr() {}

type value struct {
	ref glfs.Ref
}

func (f *value) PrettyPrint(w io.Writer) error {
	var err error
	switch f.ref.Type {
	case glfs.TypeBlob:
		_, err = fmt.Fprintf(w, "{blob %s}", f.ref.CID.String())
	case glfs.TypeTree:
		_, err = fmt.Fprintf(w, "{tree %s}", f.ref.CID.String())
	default:
		_, err = fmt.Fprintf(w, "{value %s}", f.ref.CID.String())
	}
	return err
}

func (f *value) Key() [32]byte {
	return f.ref.CID
}

func (v *value) Needs() stringsets.Set {
	return stringsets.Empty{}
}

func (v *value) isExpr() {}

func (v *value) String() string {
	sb := &strings.Builder{}
	v.PrettyPrint(sb)
	return sb.String()
}

func (c *Compiler) compileExpr(cc *compileCtx, exprPath string, x wantcfg.Expr) (Expr, error) {
	switch {
	case x.Blob != nil:
		return c.compileBlob(cc.ctx, cc.dst, *x.Blob)
	case x.Tree != nil:
		return c.compileTree(cc.ctx, cc.dst, cc.src, x.Tree)
	case x.Ref != nil:
		return c.compileRef(cc.ctx, cc.dst, cc.src, *x.Ref)
	case x.Compute != nil:
		return c.compileCompute(cc, exprPath, *x.Compute)
	case x.Selection != nil:
		if exprPath == "" {
			return nil, errors.New("cannot use selections when compiling snippet expression")
		}
		return c.compileSelection(cc, exprPath, *x.Selection)
	default:
		return nil, errors.Errorf("empty wantcfg.Expr at %s", exprPath)
	}
}

func (c *Compiler) compileCompute(cc *compileCtx, exprPath string, x wantcfg.Compute) (Expr, error) {
	inputs, err := c.compileInputs(cc, exprPath, x.Inputs)
	if err != nil {
		return nil, err
	}
	return &compute{
		Op:     wantdag.OpName(x.Op),
		Inputs: inputs,
	}, nil
}

func (c *Compiler) compileSelection(cc *compileCtx, exprPath string, x wantcfg.Selection) (Expr, error) {
	var moduleCID cadata.ID
	if err := moduleCID.UnmarshalBase64([]byte(x.Source.Module)); err != nil {
		return nil, fmt.Errorf("invalid source: %w", err)
	}

	callerPath := x.Source.CallerPath
	if callerPath == "" {
		return nil, fmt.Errorf("selection in file %q has empty callerPath", exprPath)
	}
	callerPath = glfs.CleanPath(callerPath)

	ks := SetFromQuery(callerPath, x.Query)
	var sel *selection
	if x.Source.Derived {
		if IsExprFilePath(callerPath) {
			ks = stringsets.Subtract(ks, stringsets.Union(
				stringsets.Unit(callerPath),
				stringsets.Prefix(callerPath+"/")),
			)
		}
		ks = stringsets.Simplify(ks)
		if stringsets.Equals(ks, stringsets.Empty{}) {
			return nil, fmt.Errorf("selection %v is the empty set", ks)
		}
		sel = newSelection(moduleCID, true, ks)
	} else {
		ks := SetFromQuery(callerPath, x.Query)
		ks = stringsets.Simplify(ks)
		if stringsets.Equals(ks, stringsets.Empty{}) {
			return nil, fmt.Errorf("selection %v is the empty set", ks)
		}
		sel = newSelection(moduleCID, false, ks)
	}
	return c.pickExpr(cc.ctx, cc.dst, sel, PathFrom(callerPath, x.Pick))
}

func (c *Compiler) compileBlob(ctx context.Context, dst cadata.Store, content string) (*value, error) {
	ref, err := glfs.PostBlob(ctx, dst, strings.NewReader(content))
	if err != nil {
		return nil, err
	}
	return &value{ref: *ref}, nil
}

// compileTree writes a tree defined by dst to
func (c *Compiler) compileTree(ctx context.Context, dst cadata.Store, src cadata.Getter, cfgEnts []wantcfg.TreeEntry) (*value, error) {
	var ents []glfs.TreeEntry
	for _, cfgEnt := range cfgEnts {
		var ref glfs.Ref
		switch {
		case cfgEnt.Value.Blob != nil:
			expr, err := c.compileBlob(ctx, dst, *cfgEnt.Value.Blob)
			if err != nil {
				return nil, err
			}
			ref = expr.ref
		case cfgEnt.Value.Tree != nil:
			expr, err := c.compileTree(ctx, dst, src, cfgEnt.Value.Tree)
			if err != nil {
				return nil, err
			}
			ref = expr.ref
		case cfgEnt.Value.Ref != nil:
			expr, err := c.compileRef(ctx, dst, src, *cfgEnt.Value.Ref)
			if err != nil {
				return nil, err
			}
			ref = expr.ref
		default:
			return nil, fmt.Errorf("tree literals can only contain blobs and trees: HAVE %v", cfgEnt.Value)
		}
		ents = append(ents, glfs.TreeEntry{
			Name:     cfgEnt.Name,
			FileMode: cfgEnt.Mode,
			Ref:      ref,
		})
	}
	ref, err := glfs.PostTreeSlice(ctx, dst, ents)
	if err != nil {
		return nil, err
	}
	return &value{ref: *ref}, nil
}

// compileRef compiles a Ref expression, which is essentially a noop, except for the check for referential integrity.
func (c *Compiler) compileRef(ctx context.Context, dst cadata.Store, src cadata.Getter, x glfs.Ref) (*value, error) {
	if err := c.glfs.WalkRefs(ctx, src, x, func(ref glfs.Ref) error {
		yes, err := stores.ExistsOnGet(ctx, src, ref.CID)
		if err != nil {
			return err
		}
		if !yes {
			return fmt.Errorf("while compiling ref, missing blob id=%v", ref.CID)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := glfstasks.FastSync(ctx, dst, src, x); err != nil {
		return nil, err
	}
	return &value{ref: x}, nil
}

func (c *Compiler) compileInputs(cc *compileCtx, stagePath string, xs []wantcfg.Input) (ys []computeInput, err error) {
	for _, x := range xs {
		n, err := c.compileExpr(cc, stagePath, x.From)
		if err != nil {
			return nil, err
		}
		y := computeInput{
			To:   x.To,
			From: n,
			Mode: x.Mode,
		}
		ys = append(ys, y)
	}
	return ys, nil
}

func maxToLen(ins []computeInput) (max int) {
	for _, in := range ins {
		l := len(in.To)
		if l > max {
			max = l
		}
	}
	return max
}

func IsContextDependent(x Expr) bool {
	switch x := x.(type) {
	case *compute:
		return true
	case *selection:
		return false
	case *value:
		return true
	default:
		panic(x)
	}
}
