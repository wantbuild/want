// package testwant provides a WBS for use in tests.
package testwant

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"wantbuild.io/want/src/internal/testutil"
	"wantbuild.io/want/src/want"
)

func NewSystem(t testing.TB) *want.System {
	ctx := testutil.Context(t)
	require.NoError(t, os.MkdirAll(stateDir(), 0o755))
	sys := want.New(stateDir(), runtime.GOMAXPROCS(0))
	require.NoError(t, sys.Init(ctx))
	return sys
}

func stateDir() string {
	return filepath.Join(os.TempDir(), "want")
}
