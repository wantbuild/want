package qemuops

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"blobcache.io/glfs"
	"go.brendoncarroll.net/state/cadata"
	"wantbuild.io/want/src/internal/glfsport"
	"wantbuild.io/want/src/wantcfg"
	"wantbuild.io/want/src/wantjob"
)

type virtioFSd struct {
	rootPath  string
	vhostPath string

	cmd *exec.Cmd
}

func (e *Executor) newVirtioFSd(rootPath string, vhostPath string, stdout, stderr io.Writer) *virtioFSd {
	cmd := func() *exec.Cmd {
		uid := os.Getuid()
		gid := os.Getgid()
		const maxIdRange = 1 << 16

		args := []string{
			fmt.Sprintf("--socket-path=%s", vhostPath),
			fmt.Sprintf("--shared-dir=%s", rootPath),
			"--cache=always",
			"--sandbox=namespace",

			fmt.Sprintf("--translate-uid=squash-host:0:0:%d", maxIdRange),
			fmt.Sprintf("--translate-gid=squash-host:0:0:%d", maxIdRange),
			fmt.Sprintf("--translate-uid=squash-guest:0:%d:%d", uid, maxIdRange),
			fmt.Sprintf("--translate-gid=squash-guest:0:%d:%d", gid, maxIdRange),
			//"--log-level=debug",
		}
		cmd := e.virtiofsdCmd(args...)
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		return cmd
	}()
	return &virtioFSd{
		rootPath:  rootPath,
		vhostPath: vhostPath,
		cmd:       cmd,
	}
}

func (vfsd *virtioFSd) Start() error {
	return vfsd.cmd.Start()
}

func (vfsd *virtioFSd) Export(ctx context.Context, src cadata.Getter, prefix string, root glfs.Ref) error {
	exp := &glfsport.Exporter{
		Dir:   vfsd.rootPath,
		Cache: glfsport.NullCache{},
		Store: src,
	}
	return exp.Export(ctx, root, prefix)
}

func (vfsd *virtioFSd) Import(ctx context.Context, dst cadata.PostExister, q wantcfg.PathSet) (*glfs.Ref, error) {
	var prefix string
	switch {
	case q.Prefix != nil:
		prefix = *q.Prefix
	case q.Unit != nil:
		prefix = *q.Unit
	default:
		return nil, fmt.Errorf("importing from %q not yet supported", q)
	}
	imp := glfsport.Importer{
		Dir:   vfsd.rootPath,
		Cache: glfsport.NullCache{},
		Store: dst,
	}
	return imp.Import(ctx, prefix)
}

func (vfsd *virtioFSd) awaitVhostSock(jc wantjob.Ctx) error {
	for i := 0; i < 10; i++ {
		_, err := os.Stat(vfsd.vhostPath)
		if os.IsNotExist(err) {
			jc.Infof("waiting for vhost.sock to come up")
			time.Sleep(100 * time.Millisecond)
		} else if err != nil {
			return err
		} else {
			jc.Infof("vhost.sock is up")
			return nil
		}
	}
	return fmt.Errorf("timedout waiting for %q", vfsd.vhostPath)
}

func (vfsd *virtioFSd) Close() error {
	proc := vfsd.cmd.Process
	if proc != nil {
		if err := proc.Kill(); err != nil {
			return err
		}
		_, err := proc.Wait()
		return err
	}
	return nil
}
