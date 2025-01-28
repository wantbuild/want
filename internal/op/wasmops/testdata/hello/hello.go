//go:build wasm

package main

import (
	"fmt"
	"os"
	"path"
)

func main() {
	fmt.Println("hello world")
	var ls func(p string) error
	ls = func(p string) error {
		ents, err := os.ReadDir(p)
		if err != nil {
			return err
		}
		for _, ent := range ents {
			p2 := path.Join(p, ent.Name())
			fmt.Println(p2, ent.IsDir(), ent.Type())
			if ent.IsDir() {
				ls(p2)
			}
		}
		return nil
	}
	p := "/"
	fmt.Println("ls", p)
	if err := ls(p); err != nil {
		panic(err)
	}
}
