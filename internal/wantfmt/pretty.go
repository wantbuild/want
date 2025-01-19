package wantfmt

import (
	"fmt"
	"io"
	"strconv"

	"github.com/kr/text"
	"wantbuild.io/want/lib/wantcfg"
)

// PrettyExpr pretty prints an expression.
func PrettyExpr(w io.Writer, x wantcfg.Expr) error {
	switch {
	case x.Blob != nil:
		fmt.Fprintf(w, "(blob %s)", *x.Blob)
	case x.Tree != nil:
		return prettyTree(w, x.Tree)
	case x.Ref != nil:
		ref := *x.Ref
		fmt.Fprintf(w, "{type: %v cid: %v}", ref.Type, ref.CID.String())
	case x.Selection != nil:
		sel := *x.Selection
		fmt.Fprintf(w, "(select %s %v)", sel.Source, sel.Query)
	case x.Compute != nil:
		return prettyCompute(w, *x.Compute)
	default:
		_, err := fmt.Fprintf(w, "(empty)")
		return err
	}
	return nil
}

func prettyTree(w io.Writer, tree wantcfg.Tree) error {
	fmt.Fprintf(w, "{\n")
	for k, v := range tree {
		fmt.Fprintf(w, "\t%q: {%v ", k, v.Mode)
		w2 := text.NewIndentWriter(w, []byte("  "))
		if err := PrettyExpr(w2, v.Value); err != nil {
			return err
		}
		fmt.Fprintf(w, "\n")
	}
	fmt.Fprintf(w, "}")
	return nil
}

func prettyCompute(w io.Writer, c wantcfg.Compute) error {
	switch {
	case len(c.Inputs) == 0:
		fmt.Fprintf(w, "(%s)", c.Op)
	case len(c.Inputs) == 1 && c.Inputs[0].To == "":
		in := c.Inputs[0]
		fmt.Fprintf(w, "(%s ", c.Op)
		if err := PrettyExpr(w, in.From); err != nil {
			return err
		}
		fmt.Fprintf(w, ")")
	default:
		fmt.Fprintf(w, "%s(\n", c.Op)
		w2 := text.NewIndentWriter(w, []byte("    "))
		l := maxToLen(c.Inputs)
		for _, in := range c.Inputs {
			to := in.To
			fmt.Fprintf(w2, "%-"+strconv.Itoa(l+2)+"q = ", to)
			if err := PrettyExpr(w2, in.From); err != nil {
				return err
			}
			fmt.Fprintf(w2, "\n")
		}
		fmt.Fprintf(w, ")")
	}
	return nil
}

func maxToLen(ins []wantcfg.Input) (max int) {
	for _, in := range ins {
		l := len(in.To)
		if l > max {
			max = l
		}
	}
	return max
}
