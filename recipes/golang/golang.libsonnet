local want = import "@want";
local linux = import "linux/linux.libsonnet";
local alpine = import "alpine/alpine.libsonnet";
local builtin = import "./builtin.libsonnet";

local currentVersion = "1.23.4";

local hashes = {
    "amd64-linux-1.23.4": "6924efde5de86fe277676e929dc9917d466efa02fb934197bc2eba35d5680971",
};

// dist evaluates to the golang distribution
local dist(arch, os, version=currentVersion) =
    local url = if os == "linux" then "https://go.dev/dl/go%s.%s-%s.tar.gz" % [version, os, arch];
    local hash = hashes[arch + "-" + os + "-" + version];
    want.pick(
        want.importURL(url, "SHA256", hash, ["ungzip", "untar"]),
        "go",
    );

// pathSet matches files that end in .go plus go.mod and go.sum
local pathSet = want.union([want.unit("go.mod"), want.unit("go.sum"), want.suffix(".go")]);

local modDownload(modSrc) = builtin.modDownload(
    want.filter(modSrc, want.union([want.unit("go.mod"), want.unit("go.sum")]))
);

local makeExec(modSrc, main, goarch, goos) = builtin.makeExec(modSrc, modDownload(modSrc), main, goarch, goos);

local makeTestExec(modSrc, main, goarch, goos) = builtin.makeTestExec(modSrc, modDownload(modSrc), main, goarch, goos);

local wantGoExec = makeExec(
    want.selectDir(GROUND, ""),
    "recipes/golang/wantgo_main",
    goarch="wasm", goos="wasip1",
);

local defaultKernel = linux.bzImage;

local goCmd(modSrc, cmd, basefs, kernel) = 
    local initscript = want.blob(|||
        #!/bin/bash
        set -ve;

        export PATH=$PATH:/usr/local/go/bin
        export GOROOT=/usr/local/go;
        export GOMODCACHE=/gomodcache;
        export CGO_ENABLED=0;

        cd /input;
        ls;
    ||| + "go %s | tee /output/out.txt;" % [cmd] + 
    ||| 
        reboot -f;
    |||);
    local rootfs = want.pass([
        want.input("", basefs),
        want.input("/usr/local/go", dist("amd64", "linux")),
        want.input("/initscript", initscript),
        want.input("/gomodcache", modDownload(modSrc)),
        want.input("/input", modSrc),
        want.input("/output", want.tree()),
    ]);
    linux.runVM(1, 4e9, kernel, rootfs, init="/bin/sh", args=["/initscript"], output=want.prefix("output"));

local goTest(modSrc, basefs=alpine.rootfs(alpine.ARCH_AMD64), kernel=defaultKernel) =
    local cmd = "test -v -coverprofile /output/coverage -ldflags '-s -w -buildid=' -buildvcs=false ./...";
    goCmd(modSrc, cmd, basefs, kernel);

local defaultBaseFS = want.tree({
    "tmp": want.treeEntry("777", want.tree([])),
});

local runTests(modSrc, basefs=defaultBaseFS, kernel=defaultKernel, ignore=[]) =
    local task = want.pass([
        want.input("module", modSrc),
        want.input("modcache", modDownload(modSrc)), 
        want.input("run_tests.json", want.blob(std.manifestJsonEx({
            GOOS : "linux",
            GOARCH : "amd64",
            Ignore: ignore,
        },""))),
        want.input("basefs", basefs),
        want.input("kernel", kernel),
    ]);
    want.wasm.nativeGLFS(1e9, wantGoExec, task, ["", "runTests"]);

{
    dist :: dist,
    pathSet :: pathSet,
    modDownload :: modDownload,    
    makeExec :: makeExec,
    makeTestExec :: makeTestExec,
    runTests :: runTests,
    goTest :: goTest,
}
