local want = import "want";

local qbootRom = want.importURL(
    url="https://github.com/wantbuild/qemu-static/releases/download/v0.1.0/qboot.rom",
	algo="SHA256",
	hash="9b9dfc6c25740d6225625570d71cab6805cc9216e68c8932e343266daaeb8c4b",
);

local virtiofsds = {
    "arm64-linux": want.importURL(
		url="http://mirror.archlinuxarm.org/aarch64/extra/virtiofsd-1.13.0-1-aarch64.pkg.tar.xz",
		algo="SHA256",
		transforms=["unxz", "untar"],
    ),
    "amd64-linux": want.importURL(
        url="https://london.mirror.pkgbuild.com/extra/os/x86_64/virtiofsd-1.13.0-1-x86_64.pkg.tar.zst",
		algo="SHA256",
        hash="c2ae0b587508ddac445b4eb07cf6589b3e3391b27c2e6eaeb636bc585c1e2165",
		transforms=["unzstd", "untar"],
    ),
};

local virtiofsd(arch, os) =
    local key = arch + "-" + os;
    want.pick(virtiofsds[key], "usr/lib/virtiofsd");

local qemuSystem_X86_64s = {
    "arm64-linux": want.importURL(
        url="https://github.com/wantbuild/qemu-static/releases/download/v0.1.0/qemu-system-x86_64_arm64_linux",
        algo="SHA256",
        hash="36960c7b8ed29b8bfd7bf5d538620ec9e1080ec2ac1b8e0929c5df9e47a0f5f5",
    ),
    "amd64-linux": want.importURL(
        url ="https://github.com/wantbuild/qemu-static/releases/download/v0.1.0/qemu-system-x86_64_amd64_linux",
        algo="SHA256",
        hash="50fee3a71399ab64972e048a54799c47eb990759b10e55c3adbae1be10a1dd72",
    ),
};

local qemuSystem_X86_64(arch, os) =
    local key = arch + "-" + os;
    qemuSystem_X86_64s[key];
