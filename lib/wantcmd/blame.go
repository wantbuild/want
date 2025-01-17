package wantcmd

import (
	"fmt"

	"github.com/kr/text"
	"go.brendoncarroll.net/star"
	"wantbuild.io/want/internal/wantc"
	"wantbuild.io/want/lib/want"
)

var blameCmd = star.Command{
	Metadata: star.Metadata{
		Short: "check what produces what",
	},
	Flags: []star.IParam{dbParam},
	F: func(c star.Context) error {
		ctx := c.Context
		repo, err := openRepo()
		if err != nil {
			return err
		}
		db := dbParam.Load(c)
		targets, err := want.Blame(ctx, db, repo)
		if err != nil {
			return err
		}

		w := c.StdOut
		for _, target := range targets {
			ss := wantc.SetFromQuery(target.DefinedIn, target.To)
			fmt.Fprintf(w, "%s\n", ss.String())
			w2 := text.NewIndentWriter(w, []byte("\t"))
			target.From.PrettyPrint(w2)
			fmt.Fprintln(w)
		}
		return c.StdOut.Flush()
	},
}
