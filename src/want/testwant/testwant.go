// package testwant provides a WBS for use in tests.
package testwant

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"wantbuild.io/want/src/internal/testutil"
	"wantbuild.io/want/src/want"
)

const stateDir = "/tmp/want"

func NewSystem(t testing.TB) *want.System {
	ctx := testutil.Context(t)
	require.NoError(t, os.MkdirAll(stateDir, 0o755))
	sys := want.New(stateDir, runtime.GOMAXPROCS(0))
	require.NoError(t, sys.Init(ctx))
	return sys
}
