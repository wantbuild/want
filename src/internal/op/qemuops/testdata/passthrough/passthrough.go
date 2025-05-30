package main

import (
	"log"
	"os"
	"syscall"

	"go.brendoncarroll.net/state/cadata"
	"wantbuild.io/want/src/wantjob"
	"wantbuild.io/want/src/wantjob/wanthttp"
)

func main() {
	if err := syscall.Mount("devtmpfs", "/dev", "devtmpfs", syscall.MS_NOSUID, "mode=0755"); err != nil {
		log.Fatalf("Failed to mount devfs: %v", err)
	}
	f, err := os.OpenFile("/dev/vport0p1", os.O_RDWR, 0o644)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	wanthttp.MainRW(f, func(jc wantjob.Ctx, s cadata.Getter, root []byte) wantjob.Result {
		return wantjob.Result{
			ErrCode: wantjob.OK,
			Schema:  wantjob.Schema_NoRefs,
			Root:    root,
		}
	})
}
