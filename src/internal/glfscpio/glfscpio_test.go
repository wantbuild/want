package glfscpio

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"blobcache.io/glfs"
	"github.com/cavaliergopher/cpio"
	"github.com/stretchr/testify/require"
	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/internal/testutil"
)

func TestWriteRead(t *testing.T) {
	s := stores.NewMem()
	tcs := []struct {
		Name string
		I    glfs.Ref
	}{
		{
			Name: "empty",
			I:    testutil.PostFSStr(t, s, nil),
		},
		{
			Name: "1 file",
			I:    testutil.PostFSStr(t, s, map[string]string{"foo": "foo123"}),
		},
		{
			Name: "3 files",
			I: testutil.PostFSStr(t, s, map[string]string{
				"a/b/c/d.txt": "d123",
				"a/b/c.txt":   "c123",
				"a/b.txt":     "b123",
			}),
		},
	}
	for i, tc := range tcs {
		tc := tc
		t.Run(fmt.Sprintf("%d-%s", i, tc.Name), func(t *testing.T) {
			ctx := testutil.Context(t)

			// write
			buf := bytes.Buffer{}
			require.NoError(t, Write(ctx, s, tc.I, &buf))

			ls(t, buf.Bytes())

			// read
			dst := stores.NewMem()
			out, err := Read(ctx, dst, bytes.NewReader(buf.Bytes()))
			require.NoError(t, err)

			testutil.EqualFS(t, stores.Union{dst, s}, tc.I, *out)
		})
	}
}

func ls(t testing.TB, data []byte) {
	cr := cpio.NewReader(bytes.NewReader(data))
	for {
		h, err := cr.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
		}
		t.Logf("%+v", h)
	}
}
