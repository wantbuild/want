package streammux

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"

	"golang.org/x/sync/errgroup"
	"wantbuild.io/want/src/internal/testutil"
)

func TestRoundTrip(t *testing.T) {
	ctx := testutil.Context(t)
	eg, ctx := errgroup.WithContext(ctx)

	p1, p2 := net.Pipe()
	m1, m2 := New(p1), New(p2)

	// server
	eg.Go(func() error {
		defer p1.Close()
		lis := &Listener{Mux: m1, Context: ctx}
		err := http.Serve(lis, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("hello"))
		}))
		if errors.Is(err, io.EOF) {
			return nil
		}
		return nil
	})

	// client
	eg.Go(func() error {
		defer p2.Close()
		rt := NewRoundTripper(m2)
		hc := http.Client{
			Transport: rt,
		}
		resp, err := hc.Get("http://localhost/")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		t.Log(body)
		if string(body) != "hello" {
			return fmt.Errorf("expected hello, got %s", body)
		}
		return nil
	})
	if err := eg.Wait(); err != nil {
		t.Fatal(err)
	}
}
