package wantcfg

import (
	_ "embed"
	"strings"
)

//go:embed want.libsonnet
var libWant string

func LibWant(fromPath string) string {
	const pattern = "@@CALLER_PATH@@"
	return strings.ReplaceAll(libWant, pattern, fromPath)
}
