//go:build wasm

package main

import (
	"encoding/json"
	"fmt"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/wantjob"
	"wantbuild.io/want/src/wantwasm"
)

func main() {
	wantwasm.Main(func(jc wantjob.Ctx, src cadata.Getter, input []byte) ([]byte, error) {
		ctx := jc.Context
		var x glfs.Ref
		if err := json.Unmarshal(input, &x); err != nil {
			return nil, err
		}
		id, err := jc.Dst.Post(ctx, []byte("hello world"))
		if err != nil {
			return nil, err
		}
		fmt.Println("got id", id)
		buf := make([]byte, 1000)
		n, err := jc.Dst.Get(ctx, id, buf)
		if err != nil {
			return nil, err
		}
		fmt.Println("read back", n, "bytes:", string(buf[:n]))
		return json.Marshal(x)
	})
}
