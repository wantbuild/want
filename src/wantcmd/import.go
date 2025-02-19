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
		afid, err := wbs.Import(ctx, repo)
		if err != nil {
			return err
		}
		dur := time.Since(startTime)
		af, err := wbs.ViewArtifact(ctx, *afid)
		if err != nil {
			return err
		}
		root, err := af.GLFS()
		if err != nil {
			return err
		}
		if err := printTreeRec(ctx, af.Store, c.StdOut, *root); err != nil {
			return err
		}
		fmt.Fprintf(c.StdErr, "%v\n", dur)
		return c.StdOut.Flush()
	},
}
