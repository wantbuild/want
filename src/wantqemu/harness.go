package wantqemu

import (
	"context"
	"io"
	"net/http"

	"go.brendoncarroll.net/state/cadata"
	"wantbuild.io/want/src/internal/streammux"
	"wantbuild.io/want/src/wantjob"
	"wantbuild.io/want/src/wantjob/wanthttp"
)

// MainRW connects to a wanthttp API served on rw and calls fn with a job context for the server.
func MainRW(rw io.ReadWriter, fn func(jc wantjob.Ctx, s cadata.Getter, input []byte) wantjob.Result) {
	mux := streammux.New(rw)
	hc := &http.Client{
		Transport: streammux.NewRoundTripper(mux),
	}
	wc := wanthttp.NewClient(hc, "")
	jc := wantjob.Ctx{
		Context: context.Background(),
		System:  wc,
		Dst:     wc.Store(wanthttp.CurrentStore),
	}
	input, inputStore, err := wc.GetInput(context.Background())
	if err != nil {
		panic(err)
	}
	res := fn(jc, inputStore, input)
	if err := wc.SetResult(context.Background(), res); err != nil {
		panic(err)
	}
}
