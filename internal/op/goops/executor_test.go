package goops

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/stores"
	"wantbuild.io/want/internal/testutil"
	"wantbuild.io/want/internal/wantsetup"
	"wantbuild.io/want/lib/wantjob"
)

func TestMakeExec(t *testing.T) {
	jc, s, e := setupTest(t)
	ref, err := e.MakeExec(jc, s, MakeExecTask{
		Module: testutil.PostFS(t, s, map[string][]byte{
			"go.mod": []byte("module test-module"),
			"go.sum": []byte(""),
			"main.go": []byte(`package main
				func main() {
				}
			`),
		}),
		MakeExecConfig: MakeExecConfig{
			Main:   "",
			GOOS:   "wasip1",
			GOARCH: "wasm",
		},
	})
	require.NoError(t, err)
	t.Log(ref)
}

func TestMakeTestExec(t *testing.T) {
	jc, s, e := setupTest(t)
	ref, err := e.MakeTestExec(jc, s, MakeTestExecTask{
		Module: testutil.PostFS(t, s, map[string][]byte{
			"go.mod": []byte("module test-module"),
			"go.sum": []byte(""),
			"main.go": []byte(`package main
				func main() {
				}
			`),
			"main_test.go": []byte(`package main
				import "testing"

				func TestA(t *testing.T) {
					t.Log("hello world")
				}
			`),
		}),
		MakeTestExecConfig: MakeTestExecConfig{
			Path:   "",
			GOOS:   "linux",
			GOARCH: "arm64",
		},
	})
	require.NoError(t, err)
	t.Log(ref)
}

func TestTest2JSON(t *testing.T) {
	jc, s, e := setupTest(t)
	input := testutil.PostBlob(t, s, []byte(`=== RUN   TestMakeExec
	| go [build -v -o /tmp/TestMakeExec3398608698/001/makeExec-1910584065/out -trimpath -ldflags -s -w -buildid= -buildvcs=false]
	| internal/goarch
	| internal/coverage/rtcov
	| internal/godebugs
	| internal/cpu
	| internal/abi
	| internal/goexperiment
	| internal/goos
	| runtime/internal/atomic
	| runtime/internal/math
	| runtime/internal/sys
	| internal/bytealg
	| runtime
	| test-module
		executor_test.go:32: blob xfpR4yu3
	--- PASS: TestMakeExec (0.83s)
	`))
	out, err := e.Test2JSON(jc, s, input)
	require.NoError(t, err)
	testutil.PrintFS(t, s, *out)
}

func setupTest(t testing.TB) (wantjob.Ctx, cadata.Store, *Executor) {
	ctx := testutil.Context(t)
	s := stores.NewMem()
	installDir := t.TempDir()

	jsys := wantjob.NewMem(ctx, wantsetup.NewExecutor())
	err := wantsetup.Install(ctx, jsys, installDir, InstallSnippet())
	require.NoError(t, err)

	e := NewExecutor(installDir)
	newWriter := func(string) io.Writer {
		return os.Stderr
	}
	return wantjob.Ctx{Context: ctx, Dst: s, System: jsys, Writer: newWriter}, s, e
}

func TestSetup(t *testing.T) {
	t.Log(InstallSnippet())
	_, _, e := setupTest(t)

	ents, err := os.ReadDir(e.installDir)
	require.NoError(t, err)
	t.Log(ents)
}
