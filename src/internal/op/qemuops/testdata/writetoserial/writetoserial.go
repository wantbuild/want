//go:build linux

package main

import (
	"fmt"
	"log"
	"os"
	"syscall"
)

func main() {
	// Mount devfs at /dev
	if err := syscall.Mount("devtmpfs", "/dev", "devtmpfs", syscall.MS_NOSUID, "mode=0755"); err != nil {
		log.Fatalf("Failed to mount devfs: %v", err)
	}
	fmt.Println("Successfully mounted devfs at /dev")

	// Read and print contents of /dev
	entries, err := os.ReadDir("/dev")
	if err != nil {
		log.Fatalf("Failed to read /dev directory: %v", err)
	}

	fmt.Println("\nContents of /dev")
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			fmt.Printf("- %s (failed to get info: %v)\n", entry.Name(), err)
			continue
		}
		fmt.Printf("- %s (mode: %v, size: %d bytes)\n", entry.Name(), info.Mode(), info.Size())
	}

	f, err := os.OpenFile("/dev/vport0p1", os.O_RDWR, 0o644)
	if err != nil {
		log.Fatalf("failed to open /dev/vport0p1: %v", err)
	}
	defer f.Close()
	if _, err := f.Write([]byte("hello world")); err != nil {
		log.Fatalf("failed to write to /dev/vport0p1: %v", err)
	}
	fmt.Println("Successfully wrote to /dev/vport0p1")
}
