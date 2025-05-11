//go:build linux

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"syscall"

	"wantbuild.io/want/src/internal/nnc"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatalf("nnc: %s", err)
	}
}

func run(args []string) error {
	spec, err := parseSpec(args[0])
	if err != nil {
		return err
	}
	if err := prepareMounts(spec.Mounts); err != nil {
		return err
	}
	return syscall.Exec(spec.Init, spec.Args, spec.Env)
}

func prepareMounts(mounts []nnc.MountSpec) error {
	// First, ensure we're in a new mount namespace
	if err := syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("failed to make mount namespace private: %w", err)
	}
	// Create new tmpfs root
	newRoot := "/tmp/newroot"
	if err := os.MkdirAll(newRoot, 0755); err != nil {
		log.Fatalf("mkdir new root: %v", err)
	}
	if err := syscall.Mount("tmpfs", newRoot, "tmpfs", 0, ""); err != nil {
		log.Fatalf("mount tmpfs: %v", err)
	}
	// Make old root a mount point
	putOld := newRoot + "/oldroot"
	if err := os.MkdirAll(putOld, 0755); err != nil {
		log.Fatalf("mkdir oldroot: %v", err)
	}
	// pivot_root: move / to /oldroot and make newRoot the new /
	if err := syscall.PivotRoot(newRoot, putOld); err != nil {
		log.Fatalf("pivot_root: %v", err)
	}
	if err := syscall.Chdir("/"); err != nil {
		log.Fatalf("chdir /: %v", err)
	}

	// Handle all mounts specified in the container spec
	for _, mount := range mounts {
		if err := handleMount("/oldroot", "/", mount); err != nil {
			return fmt.Errorf("failed to handle mount %s: %w", mount.Dst, err)
		}
	}

	// Unmount old root and remove it
	if err := syscall.Unmount("/oldroot", syscall.MNT_DETACH); err != nil {
		log.Fatalf("unmount oldroot: %v", err)
	}
	if err := os.RemoveAll("/oldroot"); err != nil {
		log.Fatalf("remove oldroot: %v", err)
	}
	return nil
}

func handleMount(oldRoot, newRoot string, mount nnc.MountSpec) error {
	if err := mount.Src.Validate(); err != nil {
		return err
	}
	// Create mount point if it doesn't exist
	if err := os.MkdirAll(mount.Dst, 0755); err != nil {
		return fmt.Errorf("failed to create mount point: %w", err)
	}
	dst := filepath.Join(newRoot, mount.Dst)
	switch {
	case mount.Src.TmpFS != nil:
		return syscall.Mount("", dst, "tmpfs", 0, "")
	case mount.Src.ProcFS != nil:
		return syscall.Mount("", mount.Dst, "proc", 0, "")
	case mount.Src.SysFS != nil:
		return syscall.Mount("", mount.Dst, "sysfs", 0, "")
	case mount.Src.Host != nil:
		src := filepath.Join(oldRoot, *mount.Src.Host)
		return syscall.Mount(src, dst, "", syscall.MS_BIND, "")
	default:
		panic(mount) // Validate should have caught this
	}
}

func parseSpec(x string) (*nnc.ContainerSpec, error) {
	var spec nnc.ContainerSpec
	if err := json.Unmarshal([]byte(x), &spec); err != nil {
		return nil, err
	}
	return &spec, nil
}
