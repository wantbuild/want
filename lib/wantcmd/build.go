package wantcmd

import (
	"go.brendoncarroll.net/star"
	"wantbuild.io/want/lib/want"
)

var buildCmd = star.Command{
	Metadata: star.Metadata{Short: "run a build"},
	Flags:    []star.IParam{dbParam},
	Pos:      []star.IParam{pathParam},
	F: func(c star.Context) error {
		ctx := c.Context
		repo, err := openRepo()
		if err != nil {
			return err
		}
		db := dbParam.Load(c)
		defer db.Close()
		res, err := want.Build(ctx, db, repo, "")
		if err != nil {
			return err
		}
		c.Printf("TARGETS: \n")
		for _, targ := range res.Targets {
			c.Printf("%s %s\n", targ.DefinedIn, targ.To)
		}
		if res.OutputRoot != nil {
			c.Printf("OUTPUT: %v\n", *&res.OutputRoot.CID)
		}
		return c.StdOut.Flush()
	},
}

var pathParam = star.Param[string]{
	Name:     "paths",
	Default:  star.Ptr(""),
	Repeated: true,
	Parse:    star.ParseString,
}
