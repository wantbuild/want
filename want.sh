#!/bin/sh

export CGO_ENABLED=0
go run -v ./cmd/want "$@"
