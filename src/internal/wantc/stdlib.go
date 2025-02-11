package wantc

import (
	_ "embed"
)

//go:embed want.libsonnet
var libWant string

func LibWant() string {
	return libWant
}
