local want = import "want";
local linux = import "./linux.libsonnet";

local currentVersion = "1.23.4";

local hashes = {
    "amd64-linux": "6924efde5de86fe277676e929dc9917d466efa02fb934197bc2eba35d5680971",
};

// dist evaluates to the golang distribution
local dist(arch, os, version=currentVersion) =
    local url = if os == "linux" then "https://go.dev/dl/go%s.%s-%s.tar.gz" % [version, os, arch];
    local hash = hashes[arch + "-" + os + "-" + version];
    want.importURL(url, "SHA256", hash, ["ungzip", "untar"]);

// pathSet matches files that end in .go plus go.mod and go.sum
local pathSet = want.union([want.unit("go.mod"), want.unit("go.sum"), want.suffix(".go")]);

local modDownload(modSrc) = want.golang.modDownload(
    want.filter(modSrc, want.union([want.unit("go.mod"), want.unit("go.sum")]))
);

local makeExec(modSrc, main, goarch, goos) = want.golang.makeExec(modSrc, modDownload(modSrc), main, goarch, goos);

local makeTestExec(modSrc, main, goarch, goos) = want.golang.makeTestExec(modSrc, modDownload(modSrc), main, goarch, goos);

local wantGoExec = makeExec(
    want.selectDir(want.FACT, ""),
    "recipes/golang/wantgo_main",
    goarch="wasm", goos="wasip1",
);

local defaultBaseFS = want.tree({
    "tmp": want.treeEntry("777", want.tree({})),
});

local runTests(modSrc, basefs=defaultBaseFS, ignore=[]) =
    local task = want.pass([
        want.input("module", modSrc),
        want.input("modcache", modDownload(modSrc)), 
        want.input("run_tests.json", want.blob(std.manifestJsonEx({
            GOOS : "linux",
            GOARCH : "amd64",
            Ignore: ignore,
        },""))),
        want.input("basefs", basefs),
        want.input("kernel", linux.bzImage),
    ]);
    want.wasm.nativeGLFS(1e9, wantGoExec, task, ["", "runTests"]);


{
    dist :: dist,
    pathSet :: pathSet,
    modDownload :: modDownload,    
    makeExec :: makeExec,
    makeTestExec :: makeTestExec,
    runTests :: runTests,
}
