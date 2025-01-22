package qemuops

import (
	"io"
	"os"
	"testing"

	"github.com/blobcache/glfs"
	"github.com/stretchr/testify/require"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/stores"
	"wantbuild.io/want/internal/testutil"
	"wantbuild.io/want/internal/wantsetup"
	"wantbuild.io/want/lib/wantjob"
	"wantbuild.io/want/recipes/linux"
)

var bzImage = linux.BzImage

func TestKArgBuilder(t *testing.T) {
	ka := kernelArgs{
		Console:        "hvc0",
		ClockSource:    "jiffies",
		IgnoreLogLevel: true,
		Reboot:         "t",
		Panic:          -1,
	}
	ka.VirtioFSRoot("myfs")
	require.Equal(t, "clocksource=jiffies console=hvc0 ignore_loglevel panic=-1 reboot=t root=myfs rw rootfstype=virtiofs", ka.String())
}

func TestPostGetTask(t *testing.T) {
	ctx := testutil.Context(t)
	s := stores.NewMem()

	x := MicroVMTask{
		Cores:  1,
		Memory: 1024 * 1e6,
		Kernel: testutil.PostBlob(t, s, bzImage),
		Root: testutil.PostFS(t, s, map[string][]byte{
			"a": []byte("1"),
			"b": []byte("2"),
			"c": []byte("3"),
		}),
	}

	ref, err := PostMicroVMTask(ctx, s, x)
	require.NoError(t, err)
	y, err := GetMicroVMTask(ctx, s, *ref)
	require.NoError(t, err)
	require.NotNil(t, y)
	require.Equal(t, x, *y)
}

func TestInstall(t *testing.T) {
	ctx := testutil.Context(t)
	outDir := t.TempDir()
	jsys := wantjob.NewMem(ctx, wantsetup.NewExecutor())
	require.NoError(t, wantsetup.Install(ctx, jsys, outDir, InstallSnippet()))
}

func TestMicroVM(t *testing.T) {
	jc, s, e := setupTest(t)
	kernelRef := testutil.PostBlob(t, s, bzImage)
	helloRef := testutil.PostLinuxAmd64(t, s, "./testdata/helloworld")

	out, err := e.RunMicroVM(jc, s, MicroVMTask{
		Cores:  1,
		Memory: 1024 * 1e6,
		Kernel: kernelRef,
		Root: testutil.PostTree(t, s, []glfs.TreeEntry{
			{Name: "/sbin/init", Ref: helloRef, FileMode: 0o777},
		}),
		Args: []string{"arg1", "-arg2=fasd", "-arg_3"},
	})
	t.Log(out)
	require.NoError(t, err)
	testutil.PrintFS(t, s, *out)
	testutil.PrintFile(t, s, *out, "out.txt")
}

func setupTest(t testing.TB) (wantjob.Ctx, cadata.Store, *Executor) {
	ctx := testutil.Context(t)
	s := stores.NewMem()
	installDir := t.TempDir()

	jsys := wantjob.NewMem(ctx, wantsetup.NewExecutor())
	err := wantsetup.Install(ctx, jsys, installDir, InstallSnippet())
	require.NoError(t, err)

	e := NewExecutor(installDir, 4*1e9)
	newWriter := func(_ string) io.Writer {
		return os.Stderr
	}
	return wantjob.Ctx{Context: ctx, Dst: s, System: jsys, Writer: newWriter}, s, e
}
