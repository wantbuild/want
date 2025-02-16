local want = import "@want";

{
    makeExec :: function(modSrc, modcache, main, goarch, goos)
        local config = want.blob(std.manifestJsonEx({
            "GOARCH": goarch,
            "GOOS": goos,
            "main": main,
        }, ""));
        want.compute("golang.makeExec", [
            want.input("module", modSrc),
            want.input("modcache", modcache),
            want.input("config.json", config),
        ]),
    
    modDownload :: function(x)
        want.compute("golang.modDownload", [
        want.input("", x),
    ]),
    makeTestExec :: function(modSrc, modcache, path, goarch, goos)
        local config = want.blob(std.manifestJsonEx({
            "GOARCH": goarch,
            "GOOS": goos,
            "Path": path,
        }, ""));
        want.compute("golang.makeTestExec", [
            want.input("module", modSrc),
            want.input("modcache", modcache),
            want.input("config.json", config),
        ],
    ),
}
