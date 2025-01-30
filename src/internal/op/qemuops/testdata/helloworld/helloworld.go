//go:build amd64

package main

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

func main() {
	fmt.Println("Hello World")
	fmt.Println(time.Now())

	if err := os.WriteFile("out.txt", []byte(fmt.Sprintf("%v\n%v\nhello world\n", os.Args, os.Environ())), 0o644); err != nil {
		panic(err)
	}

	fmt.Println("ReadDir /")
	ls("/")

	if err := syscall.Reboot(syscall.LINUX_REBOOT_CMD_RESTART); err != nil {
		panic(err)
	}
}

func ls(p string) {
	ents, err := os.ReadDir(p)
	if err != nil {
		panic(err)
	}
	for _, ent := range ents {
		finfo, err := ent.Info()
		if err != nil {
			panic(err)
		}
		sysinfo := finfo.Sys().(*syscall.Stat_t)
		fmt.Printf("%-16s %v uid=%v gid=%d\n", finfo.Name(), finfo.Mode(), sysinfo.Uid, sysinfo.Gid)
	}
}
