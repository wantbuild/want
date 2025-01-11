package wantcmd

import (
	"log"

	"go.brendoncarroll.net/star"
)

var buildCmd = star.Command{
	Metadata: star.Metadata{Short: "run a build"},
	Flags:    []star.IParam{dbParam},
	F: func(c star.Context) error {
		repo, err := openRepo()
		if err != nil {
			return err
		}
		db := dbParam.Load(c)
		_, root, err := importRepo(c.Context, db, repo)
		if err != nil {
			return err
		}
		log.Println(root)
		if err := glfsLs(c.Context, db, *root, c.StdOut); err != nil {
			return err
		}
		return c.StdOut.Flush()
	},
}
