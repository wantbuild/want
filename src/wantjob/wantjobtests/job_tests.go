package wantjobtests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.brendoncarroll.net/state/cadata"
	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/internal/testutil"
	"wantbuild.io/want/src/wantjob"
)

func TestJobs(t *testing.T, mksys func(t testing.TB, exec wantjob.Executor) wantjob.System) {
	t.Run("Do", func(t *testing.T) {
		ctx := testutil.Context(t)
		sys := mksys(t, wantjob.BasicExecutor{
			"toUpper": func(jc wantjob.Ctx, src cadata.Getter, data []byte) wantjob.Result {
				return *wantjob.Success(wantjob.Schema_NoRefs, []byte(strings.ToUpper(string(data))))
			},
		})
		res, _, err := wantjob.Do(ctx, sys, stores.NewVoid(), wantjob.Task{Op: "toUpper", Input: []byte("hello")})
		require.NoError(t, err)
		require.NoError(t, res.Err())
		require.Equal(t, "HELLO", string(res.Data))
	})

}
