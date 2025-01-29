//go:build wasm

package main

import (
	"io"
	"os"
	"path/filepath"
)

func main() {
	ents, err := os.ReadDir("/input")
	if err != nil {
		panic(err)
	}
	for _, ent := range ents {
		pin := filepath.Join("/input", ent.Name())
		pout := filepath.Join("/output", ent.Name())
		if err := copyFile(pout, pin); err != nil {
			panic(err)
		}
	}
}

func copyFile(dst, src string) error {
	fin, err := os.OpenFile(src, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer fin.Close()
	fout, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer fout.Close()
	if _, err := io.Copy(fout, fin); err != nil {
		return err
	}
	return fout.Close()
}
