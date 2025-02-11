local want = import "@want";

local bzImage = want.selectFile(GROUND, "./linux/bzImage");

local dumbInit = want.importURL(
    url="https://github.com/Yelp/dumb-init/releases/download/v1.2.5/dumb-init_1.2.5_x86_64",
    algo="SHA256",
    hash="e874b55f3279ca41415d290c512a7ba9d08f98041b28ae7c2acb19a545f1c4df"
);

local runVM(cores, memory, kernel, rootfs, init="/sbin/init", args=[], output=want.prefix("")) =
    local virtiofs = {
        "myfs": want.qemu.virtiofs(root=rootfs, writeable=true),
    };

    local kargs = "console=hvc0 reboot=t panic=-1 rootfstype=virtiofs root=myfs rw init=%s " % [init];
    want.qemu.amd64_microvm(cores, memory, kernel,
        initrd = null,
        kargs = kargs + std.join(" ", args),
        virtiofs = virtiofs,
        output = want.qemu.virtiofs_output("myfs", output),
    );

{
    bzImage :: bzImage,
    dumbInit :: dumbInit,
    runVM :: runVM,
}
