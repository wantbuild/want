package wantcmd

import (
	"fmt"
	"io"
	"net/http"
	"path"
	"time"

	"github.com/blobcache/glfs"
	"github.com/pkg/errors"
	"go.brendoncarroll.net/star"

	"wantbuild.io/want/src/internal/glfsiofs"
)

var buildCmd = star.Command{
	Metadata: star.Metadata{Short: "run a build"},
	Flags:    []star.IParam{},
	Pos:      []star.IParam{pathsParam},
	F: func(c star.Context) error {
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
		res, err := wbs.Build(c.Context, repo, "")
		if err != nil {
			return err
		}
		dur := time.Since(startTime)
		if res.OutputRoot != nil {
			c.Printf("%v\n", res.OutputRoot.CID)
		}
		for i, targ := range res.Targets {
			tres := res.TargetResults[i]
			if targ.IsStatement {
				c.Printf("%s %v:\n", targ.DefinedIn, targ.DefinedNum)
			} else {
				c.Printf("%s:\n", targ.DefinedIn)
			}
			c.Printf("  %v %v\n", tres.ErrCode, tres.Ref)
		}
		c.Printf("%v\n", dur)
		return c.StdOut.Flush()
	},
}

var lsCmd = star.Command{
	Metadata: star.Metadata{Short: "list tree entries in the build output"},
	Flags:    []star.IParam{},
	Pos:      []star.IParam{pathParam},
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
		p := pathParam.Load(c)
		res, err := wbs.Build(c.Context, repo, p)
		if err != nil {
			return err
		}
		src := res.Store
		ref := res.OutputRoot

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
	Flags:    []star.IParam{},
	Pos:      []star.IParam{pathsParam},
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
		ps := pathsParam.LoadAll(c)
		// TODO: only build longest common prefix
		commonPrefix := ""
		res, err := wbs.Build(c.Context, repo, commonPrefix)
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

var serveHttpCmd = star.Command{
	Metadata: star.Metadata{Short: "serve the build output over http"},
	Pos:      []star.IParam{pathParam},
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
		p := pathParam.Load(c)
		res, err := wbs.Build(ctx, repo, p)
		if err != nil {
			return err
		}
		src := res.Store
		ref := res.OutputRoot
		fsys := glfsiofs.New(src, *ref)
		laddr := "127.0.0.1:8000"
		c.Printf("http://%s\n", laddr)
		c.StdOut.Flush()

		h := http.FileServerFS(fsys)
		return http.ListenAndServe(laddr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch path.Ext(r.URL.Path) {
			case ".css":
				w.Header().Set("Content-Type", "text/css")
			case ".js":
				w.Header().Set("Content-Type", "text/javascript")
			}
			h.ServeHTTP(w, r)
		}))
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
