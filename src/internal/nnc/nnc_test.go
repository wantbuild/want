package nnc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"wantbuild.io/want/src/internal/testutil"
)

func TestRun(t *testing.T) {
	ctx := testutil.Context(t)
	execPath := setup(t)

	scratchDir := t.TempDir()
	testBin := testutil.BuildLinuxAmd64(t, "./testbin")
	require.NoError(t, os.WriteFile(filepath.Join(scratchDir, "testbin"), testBin, 0755))

	ec, err := Run(ctx, execPath, ContainerSpec{
		Init: "data1/testbin",
		Mounts: []MountSpec{
			{
				Dst: "/tmp1",
				Src: MountSrc{
					TmpFS: &struct{}{},
				},
			},
			{
				Dst: "/data1",
				Src: MountSrc{Host: &scratchDir},
			},
			{
				Dst: "/proc",
				Src: MountSrc{
					ProcFS: &struct{}{},
				},
			},
			{
				Dst: "/sys",
				Src: MountSrc{
					SysFS: &struct{}{},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 0, ec)
}

func setup(t testing.TB) string {
	nncBin := testutil.BuildLinuxAmd64(t, "./nnc_main")
	tmpDir := t.TempDir()
	p := filepath.Join(tmpDir, "nnc")
	require.NoError(t, os.WriteFile(p, nncBin, 0755))
	return p
}
