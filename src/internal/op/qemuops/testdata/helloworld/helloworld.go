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
	if err := syscall.Reboot(syscall.LINUX_REBOOT_CMD_RESTART); err != nil {
		panic(err)
	}
}
