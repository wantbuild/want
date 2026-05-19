//go:build linux

package main

import (
	"encoding/json"
	"log"
	"os"
	"syscall"

	"blobcache.io/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/recipes/golang/wantgo"
	"wantbuild.io/want/src/wantjob"
	"wantbuild.io/want/src/wantjob/wanthttp"
)

func main() {
	log.Println("args", os.Args)
	log.Println("env", os.Environ())

	if err := syscall.Mount("devtmpfs", "/dev", "devtmpfs", syscall.MS_NOSUID, "mode=0755"); err != nil {
		panic(err)
	}
	if err := syscall.Mount("proc", "/proc", "proc", syscall.MS_NOSUID, ""); err != nil {
		panic(err)
	}
	if err := syscall.Mount("sysfs", "/sys", "sysfs", syscall.MS_NOSUID, ""); err != nil {
		panic(err)
	}
	if err := syscall.Mount("tmpfs", "/tmp", "tmpfs", syscall.MS_NOSUID, ""); err != nil {
		panic(err)
	}

	apiPort, err := os.OpenFile("/dev/vport0p1", os.O_RDWR, 0o644)
	if err != nil {
		panic(err)
	}

	switch os.Args[1] {
	case "runTests":
		wanthttp.MainRW(apiPort, func(jc wantjob.Ctx, src cadata.Getter, x []byte) wantjob.Result {
			var ref glfs.Ref
			if err := json.Unmarshal(x, &ref); err != nil {
				panic(err)
			}
			task, err := wantgo.GetRunTestsTask(jc.Context, src, ref)
			if err != nil {
				panic(err)
			}
			out, err := wantgo.RunTests(jc, src, *task)
			if err != nil {
				return *wantjob.Result_ErrExec(err)
			}
			return wantjob.Result{
				ErrCode: wantjob.OK,
				Schema:  wantjob.Schema_GLFS,
				Root:    jsonMarshal(*out),
			}
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

func jsonMarshal(x any) []byte {
	bz, err := json.Marshal(x)
	if err != nil {
		panic(err)
	}
	return bz
}
