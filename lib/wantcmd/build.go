package wantcmd

import (
	"fmt"
	"io"

	"github.com/blobcache/glfs"
	"github.com/pkg/errors"
	"go.brendoncarroll.net/star"
	"wantbuild.io/want/lib/want"
)

var buildCmd = star.Command{
	Metadata: star.Metadata{Short: "run a build"},
	Flags:    []star.IParam{dbParam},
	Pos:      []star.IParam{pathsParam},
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
			c.Printf("OUTPUT: %v\n", res.OutputRoot.CID)
		}
		return c.StdOut.Flush()
	},
}

var lsCmd = star.Command{
	Metadata: star.Metadata{Short: "list tree entries in the build output"},
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
		p := pathParam.Load(c)

		// do the build
		res, err := want.Build(ctx, db, repo, "")
		if err != nil {
			return err
		}
		src := res.Store
		ref := res.OutputRoot

		// process the output
		ref, err = glfs.GetAtPath(ctx, src, *ref, p)
		if err != nil {
			return err
		}
		if ref.Type != glfs.TypeTree {
			return errors.Errorf("cannot ls non-tree: %v", ref)
		}
		tree, err := glfs.GetTree(ctx, src, *ref)
		if err != nil {
			return err
		}
		w := c.StdOut
		if err := fmtTree(w, *tree); err != nil {
			return err
		}
		return w.Flush()
	},
}

var catCmd = star.Command{
	Metadata: star.Metadata{Short: "concatenate files from the build output and write them to stdout"},
	Flags:    []star.IParam{dbParam},
	Pos:      []star.IParam{pathsParam},
	F: func(c star.Context) error {
		ctx := c.Context
		repo, err := openRepo()
		if err != nil {
			return err
		}
		db := dbParam.Load(c)
		defer db.Close()
		ps := pathsParam.LoadAll(c)

		// do the build
		res, err := want.Build(ctx, db, repo, "")
		if err != nil {
			return err
		}
		src := res.Store
		ref := res.OutputRoot

		// process the output
		w := c.StdOut
		for _, p := range ps {
			ref, err = glfs.GetAtPath(ctx, src, *ref, p)
			if err != nil {
				return err
			}
			if ref.Type != glfs.TypeBlob {
				return fmt.Errorf("cannot cat type %v", ref.Type)
			}
			r, err := glfs.GetBlob(ctx, src, *ref)
			if err != nil {
				return err
			}
			if _, err := io.Copy(w, r); err != nil {
				return err
			}
		}
		return w.Flush()
	},
}

var pathParam = star.Param[string]{
	Name:     "path",
	Repeated: false,
	Default:  star.Ptr(""),
	Parse:    star.ParseString,
}

var pathsParam = star.Param[string]{
	Name:     "path",
	Default:  star.Ptr(""),
	Repeated: true,
	Parse:    star.ParseString,
}
