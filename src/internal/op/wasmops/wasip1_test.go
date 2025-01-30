package wasmops

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/blobcache/glfs"
	"github.com/kr/text"
	"github.com/stretchr/testify/require"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/internal/testutil"
	"wantbuild.io/want/src/wantjob"
)

func TestWASIp1(t *testing.T) {
	ctx := testutil.Context(t)
	s := stores.NewMem()

	jc := NewTestJobCtx(t, ctx, s)
	fs1 := testutil.PostTree(t, s, []glfs.TreeEntry{
		{Name: "a.txt", FileMode: 0o600, Ref: testutil.PostBlob(t, s, []byte("aaaaa"))},
		{Name: "b.txt", FileMode: 0o600, Ref: testutil.PostBlob(t, s, []byte("bbbbbbbb"))},
		{Name: "c.txt", FileMode: 0o600, Ref: testutil.PostBlob(t, s, []byte("ccc"))},
	})

	tcs := []struct {
		Task   WASIp1Task
		Output glfs.Ref
		Err    error
	}{
		{
			Task: WASIp1Task{
				Program: buildWASMBin(t, "testdata/copy/copy.go"),
				Input:   fs1,
			},
			Output: fs1,
		},
	}
	for i, tc := range tcs {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			out, err := ExecWASIp1(jc, s, tc.Task)
			if tc.Err == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.Err)
			}
			if !tc.Output.CID.IsZero() {
				require.NotNil(t, out)
				t.Log("expected")
				testutil.PrintFS(t, s, tc.Output)
				t.Log("actual")
				testutil.PrintFS(t, s, *out)
				require.Equal(t, tc.Output, *out)
			}
		})
	}
}

func buildWASMBin(t testing.TB, dir string) []byte {
	outPath := filepath.Join(t.TempDir(), "main-bin")
	defer os.Remove(outPath)
	cmd := exec.Command("go", "build", "-o", outPath, dir)
	cmd.Env = []string{
		"GOOS=wasip1",
		"GOARCH=wasm",
	}
	for _, key := range []string{
		"GOPATH",
		"GOCACHE",
		"GOROOT",
		"HOME",
	} {
		if val := os.Getenv(key); val != "" {
			cmd.Env = append(cmd.Env, key+"="+val)
		}
	}
	cmdOut, err := cmd.CombinedOutput()
	if len(cmdOut) != 0 {
		t.Log("cmd out: ", string(cmdOut))
	}
	require.NoError(t, err)

	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	return data
}

func NewTestJobCtx(t testing.TB, ctx context.Context, dst cadata.Store) wantjob.Ctx {
	t.Helper()
	return wantjob.Ctx{
		Context: ctx,
		Dst:     dst,
		System:  wantjob.NewMem(ctx, wantjob.BasicExecutor{}),
		Writer: func(s string) io.Writer {
			return text.NewIndentWriter(os.Stderr, []byte(s+"|"))
		},
	}
}
