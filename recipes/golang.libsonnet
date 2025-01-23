local want = import "want";

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
local pathSet = want.union([want.single("go.mod"), want.single("go.sum"), want.suffix(".go")]);

local modDownload(modSrc) = want.golang.modDownload(
    want.filter(modSrc, want.union([want.single("go.mod"), want.single("go.sum")]))
);

local makeExec(modSrc, main, goarch, goos) = want.golang.makeExec(modSrc, modDownload(modSrc), main, goarch, goos);

local makeTestExec(modSrc, main, goarch, goos) = want.golang.makeTestExec(modSrc, modDownload(modSrc), main, goarch, goos);

{
    dist :: dist,
    pathSet :: pathSet,
    modDownload :: modDownload,    
    makeExec :: makeExec,
    makeTestExec :: makeTestExec,
}
