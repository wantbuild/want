package qemuops

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/blobcache/glfs"
	"github.com/stretchr/testify/require"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/internal/testutil"
	"wantbuild.io/want/src/internal/wantsetup"
	"wantbuild.io/want/src/wantcfg"
	"wantbuild.io/want/src/wantjob"
	"wantbuild.io/want/src/wantqemu"
)

func TestKArgBuilder(t *testing.T) {
	tcs := []struct {
		I kernelArgs
		O string
	}{
		{
			I: kernelArgs{
				Console:        "hvc0",
				ClockSource:    "jiffies",
				IgnoreLogLevel: true,
				Reboot:         "t",
				Panic:          -1,
			}.VirtioFSRoot("myfs"),
			O: "clocksource=jiffies console=hvc0 ignore_loglevel panic=-1 reboot=t root=myfs rw rootfstype=virtiofs",
		},
	}
	for i, tc := range tcs {
		tc := tc
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			require.Equal(t, tc.O, tc.I.String())
		})
	}
}

func TestConfigArgs(t *testing.T) {
	tcs := []struct {
		I vmConfig
		O []string
	}{
		{
			I: vmConfig{
				CharDevs: map[string]chardevConfig{
					"char0": {
						Backend: "socket",
						Props: map[string]string{
							"path": "test-path.socket",
						},
					},
				},
			},
			O: []string{"-chardev", "socket,id=char0,path=test-path.socket"},
		},
		{
			I: vmConfig{
				Numa: []numaConfig{
					{Type: "node", MemDev: "mem0"},
				},
			},
			O: []string{"-numa", "node,memdev=mem0"},
		},
	}
	for i, tc := range tcs {
		tc := tc
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			args := tc.I.Args(nil)
			require.Equal(t, tc.O, args)
		})
	}
}

func TestInstall(t *testing.T) {
	ctx := testutil.Context(t)
	t.SkipNow()

	outDir := t.TempDir()
	jsys := wantjob.NewMem(ctx, wantsetup.NewExecutor())
	require.NoError(t, wantsetup.Install(ctx, jsys, outDir, InstallSnippet()))
}

func TestMicroVM(t *testing.T) {
	jc, s, e := setupTest(t)

	kernelRef := testutil.PostBlob(t, s, loadKernel(t))
	helloRef := testutil.PostLinuxAmd64(t, s, "./testdata/helloworld")
	// emptyTree := testutil.PostFSStr(t, s, nil)
	kargs := kernelArgs{
		Console:        "hvc0",
		ClockSource:    "jiffies",
		IgnoreLogLevel: true,
		Reboot:         "t",
		Panic:          -1,
		RandomTrustCpu: "on",
	}

	tcs := []struct {
		Task MicroVMTask

		Output *glfs.Ref
		Err    error
	}{
		{
			Task: MicroVMTask{
				Cores:      1,
				Memory:     1024 * 1e6,
				Kernel:     kernelRef,
				KernelArgs: kargs.VirtioFSRoot("vfs1").String(),
				VirtioFS: map[string]wantqemu.VirtioFSSpec{
					"vfs1": {
						Root: testutil.PostTree(t, s, []glfs.TreeEntry{
							{Name: "/sbin/init", Ref: helloRef, FileMode: 0o755},
						}),
						Writeable: true,
					},
				},
				Output: wantqemu.GrabVirtioFS("vfs1", wantcfg.Prefix("")),
			},
			Output: ptr(testutil.PostTree(t, s, []glfs.TreeEntry{
				{Name: "/sbin/init", Ref: helloRef, FileMode: 0o755},
				{Name: "out.txt", FileMode: 0o644, Ref: testutil.PostString(t, s, "[/sbin/init]\n[HOME=/ TERM=linux]\nhello world\n")},
			})),
		},
		{
			Task: MicroVMTask{
				Cores:      1,
				Memory:     1024 * 1e6,
				Kernel:     kernelRef,
				KernelArgs: "panic=-1 console=hvc0 reboot=t",
				Initrd: ptr(testutil.PostTree(t, s, []glfs.TreeEntry{
					{Name: "init", FileMode: 0o777, Ref: helloRef},
				})),
			},
			Err: ErrInvalidOutputSpec{},
		},
	}
	for i, tc := range tcs {
		tc := tc
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			out, err := e.amd64MicroVM(jc, s, tc.Task)
			t.Log(out)
			if tc.Err != nil {
				require.ErrorIs(t, err, tc.Err)
			} else {
				require.NoError(t, err)
			}
			if tc.Output != nil {
				testutil.EqualFS(t, stores.Union{jc.Dst, s}, *tc.Output, *out)
			}
		})
	}
}

func setupTest(t testing.TB) (wantjob.Ctx, cadata.Store, *Executor) {
	ctx := testutil.Context(t)
	s := stores.NewMem()
	installDir, err := filepath.Abs("testcache")
	require.NoError(t, err)
	jsys := wantjob.NewMem(ctx, wantsetup.NewExecutor())
	e := NewExecutor(Config{InstallDir: installDir, MemLimit: 4 * 1e9})
	newWriter := func(_ string) io.Writer {
		return os.Stderr
	}
	jc := wantjob.Ctx{Context: ctx, Dst: s, System: jsys, Writer: newWriter}

	if finfo, err := os.Stat(installDir); err == nil && finfo.IsDir() {
		t.Log("testcache exists, skipping install")
		return jc, s, e
	}
	require.NoError(t, wantsetup.Install(ctx, jsys, installDir, InstallSnippet()))
	return jc, s, e
}

func loadKernel(t testing.TB) []byte {
	data, err := os.ReadFile("./bzImage")
	require.NoError(t, err)
	return data
}

func ptr[T any](x T) *T {
	return &x
}
