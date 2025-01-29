package wantcmd

import (
	"fmt"

	"github.com/kr/text"
	"go.brendoncarroll.net/star"

	"wantbuild.io/want/src/internal/wantc"
	"wantbuild.io/want/src/internal/wantfmt"
)

var blameCmd = star.Command{
	Metadata: star.Metadata{
		Short: "check what produces what",
	},
	Flags: []star.IParam{},
	F: func(c star.Context) error {
		ctx := c.Context
		wbs, err := newSys(&c)
		if err != nil {
			return err
		}
		defer wbs.Close()
		repo, err := openRepo()
		if err != nil {
			return err
		}
		targets, err := wbs.Blame(ctx, repo)
		if err != nil {
			return err
		}

		w := c.StdOut
		for _, target := range targets {
			// key
			ss := wantc.SetFromQuery(target.DefinedIn, target.To)
			fmt.Fprintf(w, "%s\n", ss.String())

			// value
			w2 := text.NewIndentWriter(w, []byte("  "))
			if err := wantfmt.PrettyExpr(w2, target.Expr); err != nil {
				return err
			}
			fmt.Fprintln(w)
		}
		return c.StdOut.Flush()
	},
}
