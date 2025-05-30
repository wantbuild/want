//go:build linux

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"syscall"

	"wantbuild.io/want/src/internal/streammux"
	"wantbuild.io/want/src/wantjob"
	"wantbuild.io/want/src/wantjob/wanthttp"
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
	mux := streammux.New(f)
	hc := &http.Client{Transport: streammux.NewRoundTripper(mux)}
	c := wanthttp.NewClient(hc, "")
	ctx := context.Background()
	if err := c.SetResult(ctx, wantjob.Result{
		ErrCode: wantjob.OK,
		Root:    []byte("hello world"),
	}); err != nil {
		log.Fatalf("failed to set result: %v", err)
	}
	fmt.Println("Successfully wrote to /dev/vport0p1")
}
