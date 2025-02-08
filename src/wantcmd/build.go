package wantcmd

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/blobcache/glfs"
	"github.com/blobcache/glfs/glfsiofs"
	"github.com/pkg/errors"
	"go.brendoncarroll.net/exp/slices2"
	"go.brendoncarroll.net/star"

	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/internal/wantc"
	"wantbuild.io/want/src/wantcfg"
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
		// query
		q := mkBuildQuery(pathsParam.LoadAll(c)...)
		res, err := wbs.Build(c.Context, repo, q)
		if err != nil {
			return err
		}
		dur := time.Since(startTime)
		if res.OutputRoot != nil {
			c.Printf("INPUT: %v\n", res.Source)
			c.Printf("QUERY: %v\n", q)
		}
		for i, targ := range res.Targets {
			tres := res.TargetResults[i]
			if targ.IsStatement {
				c.Printf("%s[%v]:\n", targ.DefinedIn, targ.DefinedNum)
			} else {
				c.Printf("%s:\n", targ.DefinedIn)
			}
			if ref, err := glfstasks.ParseGLFSRef(tres.Data); err == nil {
				c.Printf("  %v %v\n", tres.ErrCode, ref)
			} else {
				c.Printf("  %v %q\n", tres.ErrCode, tres.Data)
			}
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
		q := mkBuildQuery(p)
		res, err := wbs.Build(c.Context, repo, q)
		if err != nil {
			return err
		}
		src := res.Store
		ref := res.OutputRoot
		if ref == nil {
			return fmt.Errorf("cannot ls, errors occured in build. see want build")
		}
		ref, err = glfs.GetAtPath(ctx, src, *ref, p)
		if err != nil {
			return err
		}
		if ref.Type != glfs.TypeTree {
			return errors.Errorf("cannot ls non-tree: %v", ref)
		}
		tree, err := glfs.GetTreeSlice(ctx, src, *ref, 1e6)
		if err != nil {
			return err
		}
		w := c.StdOut
		if err := fmtTree(w, tree); err != nil {
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
		q := mkBuildQuery(pathsParam.LoadAll(c)...)
		res, err := wbs.Build(c.Context, repo, q)
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
	Flags:    []star.IParam{},
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
		q := mkBuildQuery(pathParam.Load(c))
		res, err := wbs.Build(ctx, repo, q)
		if err != nil {
			return err
		}
		src := res.Store
		ref := res.OutputRoot
		if ref == nil {
			return fmt.Errorf("error during build")
		}
		ref, err = glfs.GetAtPath(ctx, src, *ref, wantc.BoundingPrefix(q))
		if err != nil {
			return err
		}
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

var exportZipCmd = star.Command{
	Metadata: star.Metadata{Short: "export the build output to zip file"},
	Pos:      []star.IParam{pathParam},
	Flags:    []star.IParam{outParam},
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
		q := mkBuildQuery(pathParam.Load(c))
		res, err := wbs.Build(ctx, repo, q)
		if err != nil {
			return err
		}
		src := res.Store
		ref := res.OutputRoot
		if ref == nil {
			return fmt.Errorf("error during build")
		}
		ref, err = glfs.GetAtPath(ctx, src, *ref, wantc.BoundingPrefix(q))
		if err != nil {
			return err
		}
		zw := zip.NewWriter(outParam.Load(c))
		if err := zw.AddFS(glfsiofs.New(src, *ref)); err != nil {
			return err
		}
		if err := zw.Close(); err != nil {
			return err
		}
		c.Printf("%v\n", outParam.Load(c).Name())
		return nil
	},
}

func mkBuildQuery(prefixes ...string) wantcfg.PathSet {
	var q wantcfg.PathSet
	switch len(prefixes) {
	case 0:
		q = wantcfg.Prefix("")
	case 1:
		q = wantcfg.Prefix(prefixes[0])
	default:
		q = wantcfg.Union(slices2.Map(prefixes, wantcfg.Prefix)...)
	}
	return q
}

var outParam = star.Param[*os.File]{
	Name: "out",
	Parse: func(s string) (*os.File, error) {
		return os.OpenFile(s, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
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
