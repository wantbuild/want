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

local linux_virt() =
    local pkg = want.importURL(
        url="https://dl-cdn.alpinelinux.org/alpine/latest-stable/main/x86_64/linux-virt-6.12.9-r0.apk",
        algo = "SHA256",
        hash = "45f8fd4cdd385658b3db78b44213cb370bd87a085de6c50e3177b1308c06662e",
        transforms = ["ungzip", "untar"],
    );
    want.pick(pkg, "boot/vmlinuz-virt");

{
    rootfs :: rootfs,
    linux_virt :: linux_virt,
}
