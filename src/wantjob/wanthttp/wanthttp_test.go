package wanthttp

import (
	"net/http"
	"testing"

	"wantbuild.io/want/src/internal/testutil"
	"wantbuild.io/want/src/wantjob"
	"wantbuild.io/want/src/wantjob/wantjobtests"
)

func TestHTTP(t *testing.T) {
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
