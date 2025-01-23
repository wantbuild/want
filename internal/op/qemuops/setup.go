//go:build (amd64 || arm64) && linux

package qemuops

import (
	_ "embed"
	"fmt"
	"runtime"
)

//go:embed setup.jsonnet
var setupJsonnet string

// InstallSnippet returns a snippet which evaluates to the installation files
func InstallSnippet() string {
	return fmt.Sprintf(`local arch = "%s";`+"\n", runtime.GOARCH) +
		fmt.Sprintf(`local os = "%s";`+"\n", runtime.GOOS) +
		setupJsonnet
}
