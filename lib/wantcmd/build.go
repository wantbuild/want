package wantcmd

import (
	"go.brendoncarroll.net/star"
	"wantbuild.io/want/lib/want"
)

var buildCmd = star.Command{
	Metadata: star.Metadata{Short: "run a build"},
	Flags:    []star.IParam{dbParam},
	F: func(c star.Context) error {
		ctx := c.Context
		repo, err := openRepo()
		if err != nil {
			return err
		}
		db := dbParam.Load(c)
		_, err = want.Import(ctx, db, repo)
		if err != nil {
			return err
		}
		return nil
	},
}
