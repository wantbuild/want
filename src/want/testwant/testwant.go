// package testwant provides a WBS for use in tests.
package testwant

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"wantbuild.io/want/src/internal/testutil"
	"wantbuild.io/want/src/want"
)

const stateDir = "/tmp/want"

func NewSystem(t testing.TB) *want.System {
	ctx := testutil.Context(t)
	sys := want.New(stateDir, runtime.GOMAXPROCS(0))
	require.NoError(t, sys.Init(ctx))
	return sys
}
