local want = import "@want";

local alpine = import "alpine/alpine.libsonnet";
local linux = import "linux/linux.libsonnet";

local dist = want.importURL(
    url = "https://github.com/rust-lang/mdBook/releases/download/v0.4.43/mdbook-v0.4.43-x86_64-unknown-linux-musl.tar.gz",
    algo = "SHA256",
    hash = "3058914071a6f22dbd1b8ea734a96d8e86489743ae0bc8d6bbd9e923f191b038",
    transforms = ["ungzip", "untar"],
);

local mdbookExec = want.pick(dist, "mdbook");

local initscript = want.blob(|||
    set -ve

    /usr/bin/mdbook build /input -d /output

    reboot -f
|||);

// make will generate the HTML for a book.
// See https://rust-lang.github.io/mdBook/format/summary.html for more information about mdbook.
local make(bookToml, summaryMd, srcDir) = 
    local rootfs = want.pass([
        want.input("", alpine.rootfs(alpine.ARCH_AMD64)),
        want.input("initscript", initscript),
        want.input("/usr/bin", dist),
        want.input("/input/book.toml", bookToml),
        want.input("/input/src", srcDir),
        want.input("/input/src/SUMMARY.md", summaryMd),
        want.input("/output", want.tree()),
    ]);
    linux.runVM(1, 1e9, linux.bzImage, rootfs, init="/bin/sh", args=["initscript", ""], output=want.prefix("output"));

{
    make :: make,
}