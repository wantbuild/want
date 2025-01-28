//go:build wasm

package main

import (
	"context"
	"log"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"
	"wantbuild.io/want/internal/op/wasmops"
)

func main() {
	wasmops.Main(func(ctx context.Context, s cadata.GetPoster, x glfs.Ref) (*glfs.Ref, error) {
		id, err := s.Post(ctx, []byte("hello world"))
		if err != nil {
			return nil, err
		}
		log.Println("got id", id)
		buf := make([]byte, 1000)
		n, err := s.Get(ctx, id, buf)
		if err != nil {
			return nil, err
		}
		log.Println("read back", n, "bytes:", string(buf[:n]))
		return &x, nil
	})
}
