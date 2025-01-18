package wantcmd

import (
	"fmt"
	"time"

	"go.brendoncarroll.net/star"

	"wantbuild.io/want/lib/want"
)

var importCmd = star.Command{
	Metadata: star.Metadata{Short: "import the repository, then exit"},
	Flags:    []star.IParam{dbParam},
	F: func(c star.Context) error {
		ctx := c.Context
		startTime := time.Now()
		repo, err := openRepo()
		if err != nil {
			return err
		}
		db := dbParam.Load(c)
		srcid, err := want.Import(ctx, db, repo)
		if err != nil {
			return err
		}
		dur := time.Since(startTime)
		root, s, err := want.AccessSource(ctx, db, srcid)
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
