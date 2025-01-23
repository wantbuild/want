local want = import "want";

local hashes = {
    "amd64-linux": "6924efde5de86fe277676e929dc9917d466efa02fb934197bc2eba35d5680971",
};

local goDist(goVersion, goos, goarch) =
	local url = "https://go.dev/dl/go%s.%s-%s.tar.gz" % [goVersion, goos, goarch];
    want.pick(
        want.importURL(
            url=        url,
		    algo=       "SHA256",
		    hash=       hashes[goarch + "-" + goos],
            transforms = ["ungzip", "untar"],
        ), "go"
    );

goDist(goVersion, goos, goarch)
