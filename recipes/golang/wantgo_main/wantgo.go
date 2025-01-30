//go:build wasm

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/recipes/golang/wantgo"
	"wantbuild.io/want/src/wantjob"
	"wantbuild.io/want/src/wantwasm"
)

func main() {
	switch os.Args[1] {
	case "runTests":
		log.Println("args", os.Args)
		log.Println("env", os.Environ())

		wantwasm.Main(func(jc wantjob.Ctx, src cadata.Getter, x []byte) ([]byte, error) {
			xref, err := parseGLFS(x)
			rtt, err := wantgo.GetRunTestsTask(jc.Context, src, *xref)
			if err != nil {
				return nil, fmt.Errorf("wantgo_main, while parsing input: %w", err)
			}
			out, err := wantgo.RunTests(jc, src, *rtt)
			if err != nil {
				return nil, err
			}
			return json.Marshal(*out)
		})
	default:
		log.Println("unrecognized command: ", os.Args[1])
		os.Exit(1)
	}
}

func parseGLFS(x []byte) (*glfs.Ref, error) {
	var ret glfs.Ref
	if err := json.Unmarshal(x, &ret); err != nil {
		return nil, err
	}
	return &ret, nil
}
