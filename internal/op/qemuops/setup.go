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
	return setupJsonnet +
		"\n" +
		fmt.Sprintf(`local arch = "%s";`+"\n", runtime.GOARCH) +
		fmt.Sprintf(`local os = "%s";`+"\n", runtime.GOOS) +
		`want.pass([
	    	want.input("share/qboot.rom", qbootRom),
	    	want.input("qemu-system-x86_64", qemuSystem_X86_64(arch, os)),
	    	want.input("virtiofsd", virtiofsd(arch, os)),
		])`
}
