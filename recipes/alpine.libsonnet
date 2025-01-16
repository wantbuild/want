local want = import "want";

local hashes = {
    "alpine-minirootfs-3.21.2-x86_64" : "4aa3bd4a7ef994402f1da0f728abc003737c33411ff31d5da2ab2c3399ccbc5f",
};

local rootfs(arch, version) =
    local key = "alpine-minirootfs-" + version + "-"+ arch ;
    local url = "http://dl-cdn.alpinelinux.org/alpine/latest-stable/releases/" + arch + "/" + key + ".tar.gz";
    want.importURL(
        url = url,
        algo = "SHA256",
        hash = hashes[key],
        transforms = ["ungzip", "untar"],
    );

{
    rootfs :: rootfs,
}
