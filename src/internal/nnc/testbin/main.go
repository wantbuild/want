package main

import (
	"fmt"
	"net"
	"os"
)

func main() {
	fmt.Println("Hello, World!")

	fmt.Println("ENV", os.Environ())
	fmt.Println("UID", os.Getuid())
	fmt.Println("GID", os.Getgid())
	fmt.Println("PID", os.Getpid())

	fmt.Println("NET")
	ifs, err := net.Interfaces()
	if err != nil {
		panic(err)
	}
	for _, ifi := range ifs {
		fmt.Println("  ", ifi.Index, ifi.Name, ifi.Flags, ifi.HardwareAddr)
	}

	ls("/")
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	fmt.Println("WORKDIR:", wd)
	fmt.Println("CHDIR:", os.Chdir("./.."))
	wd, err = os.Getwd()
	if err != nil {
		panic(err)
	}
	fmt.Println("WORKDIR:", wd)
	ls("/")
}

func ls(path string) {
	fmt.Println("READDIR", path)
	ents, err := os.ReadDir(path)
	if err != nil {
		panic(err)
	}
	for _, ent := range ents {
		fmt.Println("  ", ent.Name(), ent.Type())
	}
}
