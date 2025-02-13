local want = import "@want";

local bzImage = want.selectFile(GROUND, "./bzImage");

local bzImage = want.importURL(
    url = "https://github.com/wantbuild/qemu-static/releases/download/v0.1.0/bzImage",
    algo = "SHA256",
    hash = "ae85807554a95b1e38d52c9a0e6380f7db0bf22da9a3347f23849a57ea274d15",
);

local dumbInit = want.importURL(
    url="https://github.com/Yelp/dumb-init/releases/download/v1.2.5/dumb-init_1.2.5_x86_64",
    algo="SHA256",
    hash="e874b55f3279ca41415d290c512a7ba9d08f98041b28ae7c2acb19a545f1c4df"
);

local runVM(cores, memory, kernel, rootfs, init="/sbin/init", args=[], output=want.prefix("")) =
    local virtiofs = {
        "myfs": want.qemu.virtiofs(root=rootfs, writeable=true),
    };

    local kargs = "console=hvc0 reboot=t panic=-1 rootfstype=virtiofs root=myfs rw init=%s " % [init] + std.join(" ", args);
    want.qemu.amd64_microvm(cores, memory, kernel,
        initrd = null,
        kargs = kargs,
        virtiofs = virtiofs,
        output = want.qemu.virtiofs_output("myfs", output),
    );

{
    bzImage :: bzImage,
    dumbInit :: dumbInit,
    runVM :: runVM,
}
