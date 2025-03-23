# Want Build System

Want is a hermetic build system, configured with [Jsonnet](https://jsonnet.org/).
In Want, build steps are functions from filesystem snapshots, to filesystem snapshots.
Want calls these immutable snapshots *Trees*.
Want build steps range from simple filesystem manipulation and filtering, to WebAssembly and AMD64 VMs defined with *Trees*.

For more information, read the docs available in [`doc/`](./doc/00_Introduction.md) and published at [doc.wantbuild.io](https://doc.wantbuild.io).

## Getting Started

### Download a Binary
Want can be installed by downloading a binary from the release page and putting it anywhere on your path

### Install with the Go Tool
If you are famililar with the Go tool, and `$GOPATH/bin` is on your path, you can install with `go install`.

```shell
go install wantbuild.io/want/cmd/want@latest
```

### Next Steps
Follow the guide [here](https://doc.wantbuild.io/10_Using_Want) to start using Want.

