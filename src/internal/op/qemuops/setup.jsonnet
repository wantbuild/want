local want = import "@want";

local qbootRom = want.importURL(
    url="https://github.com/wantbuild/qemu-static/releases/download/v0.1.0/qboot.rom",
	algo="SHA256",
	hash="9b9dfc6c25740d6225625570d71cab6805cc9216e68c8932e343266daaeb8c4b",
);

local virtiofsds = { 
    "amd64-linux": want.pick(want.importURL(
        url = "https://github.com/wantbuild/qemu-static/releases/download/v0.1.0/virtiofsd-c0f7439d_amd64_linux.zip",
		algo="SHA256",
        hash="124acbe163b1dfe62fcfde96b7145f0b6046521ec836fc6134f6a9cde0c2e170",
		transforms=["unzip"],
    ), "target/x86_64-unknown-linux-musl/release/virtiofsd"),
};

local virtiofsd(arch, os) =
    local key = arch + "-" + os;
    virtiofsds[key];

local qemuSystem_X86_64s = { 
    "amd64-linux": want.importURL(
        url ="https://github.com/wantbuild/qemu-static/releases/download/v0.1.0/qemu-system-x86_64_amd64_linux",
        algo="SHA256",
        hash="50fee3a71399ab64972e048a54799c47eb990759b10e55c3adbae1be10a1dd72",
    ),
};

local qemuSystem_X86_64(arch, os) =
    local key = arch + "-" + os;
    qemuSystem_X86_64s[key];

want.pass([
   	want.input("share/qboot.rom", qbootRom),
   	want.input("qemu-system-x86_64", qemuSystem_X86_64(arch, os)),
   	want.input("virtiofsd", virtiofsd(arch, os)),
])