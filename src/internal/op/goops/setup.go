//go:build !wasm

package goops

import (
	_ "embed"
	"fmt"
	"runtime"
)

//go:embed setup.jsonnet
var setupJsonnet string

func InstallSnippet() string {
	return fmt.Sprintf(`local goarch = "%s";`, runtime.GOARCH) + "\n" +
		fmt.Sprintf(`local goos = "%s";`, runtime.GOOS) + "\n" +
		fmt.Sprintf(`local goVersion = "%s";`, goVersion) + "\n" +
		setupJsonnet
}
