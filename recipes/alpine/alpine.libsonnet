local want = import "@want";

local ARCH_AMD64 = "x86_64";

local hashes = {
    "alpine-minirootfs-3.23.4-x86_64": "85498865362aa7ebececa0d725a2f2e4db7ac4e4b2850b8df21645afa0d03ee3",
};

// rootfs evlauates to the minirootfs used for containers
local rootfs(arch, version="3.23.4") =
    local key = "alpine-minirootfs-" + version + "-"+ arch ;
    local url = "http://dl-cdn.alpinelinux.org/alpine/latest-stable/releases/" + arch + "/" + key + ".tar.gz";
    want.importURL(
        url = url,
        algo = "SHA256",
        hash = hashes[key],
        transforms = ["ungzip", "untar"],
    );

// apk retrieves the contents of an apk formatted package
local apk(repo, arch, name, ver, hash) =
    local pkg = want.importURL(
        url="https://dl-cdn.alpinelinux.org/alpine/v3.23/%s/%s/%s-%s.apk" % [repo, arch, name, ver],
        algo = "SHA256",
        hash = hash,
        transforms = ["ungzip", "untar"],
    );
    want.filter(pkg, want.not(want.union([
        want.unit(".PKGINFO"),
        want.prefix(".SIGN")
    ])));

local linux_virt() =
    local x = apk("main", ARCH_AMD64, "linux-virt", "6.12.13-r0", "adf620b4ae4c9314242cc94ea3e1356cdd08d6122e79c901bc3f056723456e83");
    want.pick(x, "boot/vmlinuz-virt");

local pkg(repo, arch, name, ver, hash) = {repo: repo, arch: arch, name: name, ver: ver, hash: hash};

local apkList(pkgs) =
    want.pass(std.map(function(pkg) want.input("", apk(pkg.repo, pkg.arch, pkg.name, pkg.ver, pkg.hash)), pkgs));

{
    rootfs :: rootfs,
    linux_virt :: linux_virt,
    apk :: apk,
    pkg :: pkg,
    apkList :: apkList,

    ARCH_AMD64 :: ARCH_AMD64,
}
