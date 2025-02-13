package wantcmd

import (
	"fmt"
	"time"

	"go.brendoncarroll.net/star"
)

var importRepoCmd = star.Command{
	Metadata: star.Metadata{Short: "import the repository, then exit"},
	Flags:    []star.IParam{},
	F: func(c star.Context) error {
		ctx := c.Context
		startTime := time.Now()
		wbs, err := newSys(&c)
		if err != nil {
			return err
		}
		defer wbs.Close()
		repo, err := openRepo()
		if err != nil {
			return err
		}
		srcid, err := wbs.Import(ctx, repo)
		if err != nil {
			return err
		}
		dur := time.Since(startTime)
		root, s, err := wbs.AccessSource(ctx, srcid)
		if err != nil {
			return err
		}
		if err := printTreeRec(ctx, s, c.StdOut, *root); err != nil {
			return err
		}
		fmt.Fprintf(c.StdErr, "%v\n", dur)
		return c.StdOut.Flush()
	},
}
