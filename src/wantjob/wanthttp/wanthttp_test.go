package wanthttp

import (
	"errors"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"wantbuild.io/want/src/internal/streammux"
	"wantbuild.io/want/src/internal/testutil"
	"wantbuild.io/want/src/wantjob"
	"wantbuild.io/want/src/wantjob/wantjobtests"
)

// TestJobs runs the wantjob test suite over the HTTP API.
func TestJobs(t *testing.T) {
	wantjobtests.TestJobs(t, func(t testing.TB, exec wantjob.Executor) wantjob.System {
		ctx := testutil.Context(t)
		lis := testutil.Listen(t)
		go func() {
			srv := NewServer(wantjob.NewMem(ctx, exec))
			http.Serve(lis, srv)
		}()
		u := "http://" + lis.Addr().String()
		t.Logf("wanthttp server: %s", u)
		return NewClient(nil, u)
	})
}

func TestJobsOnPipe(t *testing.T) {
	wantjobtests.TestJobs(t, func(t testing.TB, exec wantjob.Executor) wantjob.System {
		ctx := testutil.Context(t)
		p1, p2 := net.Pipe()
		go func() {
			lis := &streammux.Listener{Mux: streammux.New(p1), Context: ctx}
			srv := NewServer(wantjob.NewMem(ctx, exec))
			if err := http.Serve(lis, srv); err != nil && !errors.Is(err, net.ErrClosed) {
				t.Logf("http.Serve: %v", err)
			}
		}()
		m2 := streammux.New(p2)
		return NewClient(&http.Client{Transport: streammux.NewRoundTripper(m2)}, "")
	})
}

func TestGetTask(t *testing.T) {
	ctx := testutil.Context(t)
	client, srv := setup(t)
	srv.task = &Task{
		Op:    "test",
		Input: []byte("test"),
	}
	task, err := client.GetTask(ctx)
	require.NoError(t, err)

	require.Equal(t, task, srv.task)
}

func TestSetResult(t *testing.T) {
	ctx := testutil.Context(t)
	client, srv := setup(t)
	client.SetResult(ctx, Result{
		ErrCode: wantjob.OK,
		Schema:  wantjob.Schema_NoRefs,
		Data:    []byte("test"),
	})
	result := Result{
		ErrCode: wantjob.OK,
		Schema:  wantjob.Schema_NoRefs,
		Data:    []byte("test"),
	}
	require.NoError(t, client.SetResult(ctx, result))
	require.Equal(t, result, *srv.result)
}

func setup(t testing.TB) (*Client, *Server) {
	ctx := testutil.Context(t)
	lis := testutil.Listen(t)
	srv := NewServer(wantjob.NewMem(ctx, nil))
	go func() {
		if err := http.Serve(lis, srv); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Logf("http.Serve: %v", err)
		}
	}()
	u := "http://" + lis.Addr().String()
	t.Logf("wanthttp server: %s", u)

	client := NewClient(nil, u)
	return client, srv
}
