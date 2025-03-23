package wantwasm

import (
	"testing"

	"blobcache.io/glfs"
	"github.com/stretchr/testify/require"
	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/internal/testutil"
)

func TestPostGetWASIp1Task(t *testing.T) {
	ctx := testutil.Context(t)
	glfsAg := glfs.NewAgent()
	s := stores.NewMem()
	//wasmBytes := buildWASMBin(t, "testdata/hello/hello.go")
	wasmBytes := []byte("fake data")
	inputRef, err := glfsAg.PostTreeSlice(ctx, s, nil)
	require.NoError(t, err)
	task := WASIp1Task{
		Program: wasmBytes,
		Input:   *inputRef,
	}
	tref, err := PostWASIp1Task(ctx, glfsAg, s, task)
	require.NoError(t, err)
	t.Log(tref)

	task2, err := GetWASIp1Task(ctx, glfsAg, s, *tref)
	require.NoError(t, err)
	require.Equal(t, task, *task2)
}
