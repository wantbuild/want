local want = import "@want";

local dist() =
    want.filter(
        want.pick(
            want.importURL(
                url = "https://nodejs.org/dist/v22.14.0/node-v22.14.0-linux-x64.tar.xz",
                algo = "SHA256",
                hash = "69b09dba5c8dcb05c4e4273a4340db1005abeafe3927efda2bc5b249e80437ec",
                transforms = ["unxz", "untar"],
            ),
            "node-v22.14.0-linux-x64"
        ),
        want.union([
            want.prefix("bin"),
            want.prefix("include"),
            want.prefix("lib"),
            want.prefix("share"),
        ])
    );

{
    dist :: dist,
}