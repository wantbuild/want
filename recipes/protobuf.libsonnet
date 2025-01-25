local want = import "want";
local linux = import "./linux.libsonnet";
local alpine = import "./alpine.libsonnet";
local golang = import "./golang.libsonnet";

local dls = {
    "amd64-linux": want.importURL(
        url="https://github.com/protocolbuffers/protobuf/releases/download/v24.4/protoc-24.4-linux-x86_64.zip",
        algo="SHA256",
        hash="5871398dfd6ac954a6adebf41f1ae3a4de915a36a6ab2fd3e8f2c00d45b50dec",
        transforms=["unzip"],
    ),
};

local dist(arch="amd64", os="linux") =
    dls[arch + "-" + os];

local genGoSrc = want.importGit(
    url="https://github.com/protocolbuffers/protobuf-go",
    commitHash="68463f0e96c93bc19ef36ccd3adfe690bfdb568c",
);

local protocGenGo = golang.makeExec(genGoSrc, "cmd/protoc-gen-go", goarch="amd64", goos="linux");

local goGrpcSrc = want.pick(
    want.importGit(
        url="https://github.com/grpc/grpc-go",
        commitHash="7765221f4bf6104973db7946d56936cf838cad46",
    ),
    "cmd/protoc-gen-go-grpc"
);

local protocGenGoGrpc = golang.makeExec(goGrpcSrc, "", goarch="amd64", goos="linux");

local kernel = linux.bzImage;

local compileGo(src) =
    local initScript = want.blob(|||
        #!/bin/sh
        set -ve;
        cd /root/

        export PATH=$PATH:/root/bin/
        protoc -I/input/ --go_out=/output --go_opt=paths=source_relative --go-grpc_out=/output --go-grpc_opt=paths=source_relative /input/*.proto
        exit;
    |||);
    local root = want.pass([
        want.input("", alpine.rootfs(alpine.ARCH_AMD64)),
        want.input("/sbin/init", initScript),
        want.input("/root", dist(arch="amd64", os="linux")),
        want.input("/root/bin/protoc-gen-go", protocGenGo, mode="777"),
        want.input("/root/bin/protoc-gen-go-grpc", protocGenGoGrpc, mode="777"),
        want.input("/input", src),
        want.input("/output", want.tree({})),
    ]);
    want.qemu.amd64_microvm_virtiofs(1, 1e9, kernel, root, init=null, args=[], output="/output");

{
    dist: dist,

    compileGo: compileGo,
}