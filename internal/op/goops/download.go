package goops

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/glfsport"
	"wantbuild.io/want/lib/wantjob"
)

func (e *Executor) ModDownload(jc wantjob.Ctx, s cadata.Getter, x glfs.Ref) (*glfs.Ref, error) {
	ctx := jc.Context
	if _, err := glfs.GetAtPath(ctx, s, x, "go.sum"); err != nil {
		return nil, err
	}
	dir, cleanup, err := e.mkdirTemp(ctx, "modDownload-")
	if err != nil {
		return nil, err
	}
	defer cleanup()
	exp := glfsport.Exporter{
		Cache: glfsport.NullCache{},
		Dir:   dir,
		Store: s,
	}
	if err := exp.Export(ctx, x, "in"); err != nil {
		return nil, err
	}

	// first pull into the executor's cache
	cmd1 := e.newCommand(ctx, goConfig{
		GOOS:       runtime.GOARCH,
		GOARCH:     runtime.GOOS,
		GOPROXY:    "direct",
		GOMODCACHE: filepath.Join(e.installDir, "gomodcache"),
	}, "mod", "download", "-x")
	cmd1.Dir = filepath.Join(dir, "in")
	cmd1.Stdout = jc.Writer("stdout")
	cmd1.Stderr = jc.Writer("stderr")
	jc.Infof("dir: %v", dir)
	jc.Infof("args: %v", cmd1.Args)
	if err := cmd1.Run(); err != nil {
		return nil, err
	}

	// then copy into specific gomodcache
	cmd2 := e.newCommand(ctx, goConfig{
		GOOS:   runtime.GOARCH,
		GOARCH: runtime.GOOS,

		GOMODCACHE: filepath.Join(dir, "modcache"),
		GOPROXY:    fmt.Sprintf("file:///%s", filepath.Join(e.installDir, "gomodcache", "cache", "download")),
	}, "mod", "download", "-x")
	cmd2.Dir = filepath.Join(dir, "in")
	cmd2.Stdout = jc.Writer("stdout")
	cmd2.Stderr = jc.Writer("stderr")
	jc.Infof("dir: %v", dir)
	jc.Infof("args: %v", cmd2.Args)
	if err := cmd2.Run(); err != nil {
		return nil, err
	}
	jc.Infof("importing modcache: begin")
	imp := glfsport.Importer{
		Cache: glfsport.NullCache{},
		Dir:   dir,
		Store: jc.Dst,
		Filter: func(x string) bool {
			return x == "modcache" || strings.HasPrefix(x, "modcache/cache")
		},
	}
	out, err := imp.Import(ctx, "modcache")
	if err != nil {
		return nil, err
	}
	jc.Infof("importing modcache: done")
	return out, nil
}
