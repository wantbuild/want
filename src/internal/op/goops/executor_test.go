package goops

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/internal/testutil"
	"wantbuild.io/want/src/internal/wantsetup"
	"wantbuild.io/want/src/wantjob"
)

func TestMakeExec(t *testing.T) {
	jc, s, e := setupTest(t, true)
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
	jc, s, e := setupTest(t, true)
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
	jc, s, e := setupTest(t, true)
	input := testutil.PostBlob(t, s, []byte(`=== RUN   TestMakeExec
	| go [build -v -o /tmp/TestMakeExec3398608698/001/makeExec-1910584065/out -trimpath -ldflags -s -w -buildid= -buildvcs=false]
	| src/internal/goarch
	| src/internal/coverage/rtcov
	| src/internal/godebugs
	| src/internal/cpu
	| src/internal/abi
	| src/internal/goexperiment
	| src/internal/goos
	| runtime/src/internal/atomic
	| runtime/src/internal/math
	| runtime/src/internal/sys
	| src/internal/bytealg
	| runtime
	| test-module
		executor_test.go:32: blob xfpR4yu3
	--- PASS: TestMakeExec (0.83s)
	`))
	out, err := e.Test2JSON(jc, s, input)
	require.NoError(t, err)
	testutil.PrintFS(t, s, *out)
}

func setupTest(t testing.TB, useExisting bool) (wantjob.Ctx, cadata.Store, *Executor) {
	ctx := testutil.Context(t)
	s := stores.NewMem()
	installDir := filepath.Join(os.TempDir(), "want-test-goroot")

	jsys := wantjob.NewMem(ctx, wantsetup.NewExecutor())
	if _, err := os.Stat(installDir); err != nil && !os.IsNotExist(err) {
		require.NoError(t, err)
	} else if err != nil || !useExisting {
		t.Log("installing into", installDir)
		require.NoError(t, os.RemoveAll(installDir))
		require.NoError(t, os.MkdirAll(installDir, 0o755))
		err := wantsetup.Install(ctx, jsys, installDir, InstallSnippet())
		require.NoError(t, err)
	} else {
		t.Log("using existing goroot", installDir)
	}

	newWriter := func(string) io.Writer {
		return os.Stderr
	}
	e := NewExecutor(installDir, filepath.Join(os.TempDir(), "want-test-scratch"))
	return wantjob.Ctx{Context: ctx, Dst: s, System: jsys, Writer: newWriter}, s, e
}

func TestSetup(t *testing.T) {
	t.Log(InstallSnippet())
	_, _, e := setupTest(t, false)

	ents, err := os.ReadDir(e.goRoot)
	require.NoError(t, err)
	t.Log(ents)
}
