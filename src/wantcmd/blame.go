package wantcmd

import (
	"fmt"

	"go.brendoncarroll.net/star"

	"wantbuild.io/want/src/internal/wantc"
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
			if target.IsStatement {
				fmt.Fprintf(w, "  RULE: %s[%d]\n", target.DefinedIn, target.DefinedNum)
			} else {
				fmt.Fprintf(w, "  RULE: %s\n", target.DefinedIn)
			}
			fmt.Fprintf(w, "  DAG: %s\n", target.DAG.CID)
		}
		return w.Flush()
	},
}
