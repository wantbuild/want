#!/bin/sh
set -ve
export CGO_ENABLED=0
go test ./...
