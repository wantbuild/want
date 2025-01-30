package wasmops_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"wantbuild.io/want/src/internal/op/wasmops"
	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/internal/testutil"
	"wantbuild.io/want/src/wantwasm"
)

func TestHarness(t *testing.T) {
	ctx := testutil.Context(t)
	s := stores.NewMem()
	e := wasmops.NewExecutor()
	jc := wasmops.NewTestJobCtx(t, ctx, s)

	out, err := e.ExecNativeGLFS(jc, s, wantwasm.NativeGLFSTask{
		Args:    []string{""},
		Program: buildWASMBin(t, "testdata/harness/harness.go"),
		Input: testutil.PostFS(t, s, map[string][]byte{
			"a.txt": []byte("hello world\n"),
		}),
	})
	require.NoError(t, err)
	t.Log(out)
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
