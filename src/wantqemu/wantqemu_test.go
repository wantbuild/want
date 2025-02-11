package wantqemu

import (
	"testing"

	"github.com/stretchr/testify/require"

	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/internal/testutil"
)

func TestPostGetMicroVMTask(t *testing.T) {
	ctx := testutil.Context(t)
	s := stores.NewMem()

	x := MicroVMTask{
		Cores:  1,
		Memory: 1024 * 1e6,
		Kernel: testutil.PostBlob(t, s, []byte("kernel bytes")),
		VirtioFS: map[string]VirtioFSSpec{
			"fs1": {
				Writeable: true,
				Root: testutil.PostFS(t, s, map[string][]byte{
					"a": []byte("1"),
					"b": []byte("2"),
					"c": []byte("3"),
				}),
			},
		},
	}

	ref, err := PostMicroVMTask(ctx, s, x)
	require.NoError(t, err)
	y, err := GetMicroVMTask(ctx, s, *ref)
	require.NoError(t, err)
	require.NotNil(t, y)
	require.Equal(t, x, *y)
}
