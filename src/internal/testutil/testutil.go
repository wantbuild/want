package testutil

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

type testKey struct {
	T *testing.T
	B *testing.B
}

func newTestKey(x testing.TB) testKey {
	switch x := x.(type) {
	case *testing.T:
		return testKey{T: x}
	case *testing.B:
		return testKey{B: x}
	default:
		panic(x)
	}
}

var ctxs = map[testKey]context.Context{}

func Context(t testing.TB) context.Context {
	k := newTestKey(t)
	ctx, exists := ctxs[k]
	if !exists {
		ctx = context.Background()
		var cf context.CancelFunc
		ctx, cf = context.WithCancel(ctx)
		t.Cleanup(cf)
		ctxs[k] = ctx
	}
	return ctx
}

func Listen(t testing.TB) net.Listener {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, lis.Close())
	})
	return lis
}
