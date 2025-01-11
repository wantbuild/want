package wantrepo

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitOpen(t *testing.T) {
	dir := t.TempDir()
	isRepo, err := IsRepo(dir)
	require.NoError(t, err)
	if isRepo {
		t.Error("should not already be a repo")
	}
	require.NoError(t, Init(dir, "TestProject"))
	isRepo, err = IsRepo(dir)
	require.NoError(t, err)
	if !isRepo {
		t.Error("should be a repo now")
	}
	r, err := Open(dir)
	require.NoError(t, err)
	t.Log(r.RawConfig())
}
