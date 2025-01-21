package testutil

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/blobcache/glfs"
	"github.com/stretchr/testify/require"
	"go.brendoncarroll.net/state/cadata"
)

func BuildLinuxAmd64(t testing.TB, srcDir string) []byte {
	outPath := filepath.Join(t.TempDir(), "main-bin")
	defer os.Remove(outPath)
	cmd := exec.Command("go", "build",
		"-o", outPath,
		srcDir)
	cmd.Env = []string{
		"GOOS=linux",
		"GOARCH=amd64",
		"CGO_ENABLED=0",
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

func PostLinuxAmd64(t testing.TB, s cadata.Store, p string) glfs.Ref {
	bin := BuildLinuxAmd64(t, p)
	ctx := Context(t)
	ref, err := glfs.PostBlob(ctx, s, bytes.NewReader(bin))
	require.NoError(t, err)
	return *ref
}
