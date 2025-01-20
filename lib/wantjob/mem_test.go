package wantjob

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/stores"
	"wantbuild.io/want/internal/testutil"
)

func TestMemJob(t *testing.T) {
	ctx := testutil.Context(t)
	sys := NewMem(ctx, BasicExecutor{
		"toUpper": func(jc Ctx, src cadata.Getter, data []byte) ([]byte, error) {
			return []byte(strings.ToUpper(string(data))), nil
		},
	})
	res, _, err := Do(ctx, sys, stores.NewVoid(), Task{Op: "toUpper", Input: []byte("hello")})
	require.NoError(t, err)
	require.NoError(t, res.Err())
	require.Equal(t, "HELLO", string(res.Data))
}
