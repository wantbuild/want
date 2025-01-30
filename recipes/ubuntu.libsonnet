local want = import "want";

local hashes = {
    "ubuntu-base-22.04-base-amd64": "df6fe77cee11bd216ac532f0ee082bdc4da3c0cc1f1d9cb20f3f743196bc4b07",
};

local ARCH_AMD64 = "amd64";

local rootfs(arch=ARCH_AMD64) =
    local key = "ubuntu-base-22.04-base-" + arch;
    local url = "https://cdimage.ubuntu.com/ubuntu-base/releases/22.04/release/" + key + ".tar.gz";
    want.importURL(url, "SHA256", hashes[key], ["ungzip", "untar"]);

{
    ARCH_AMD64 :: ARCH_AMD64,
    rootfs :: rootfs,
}
