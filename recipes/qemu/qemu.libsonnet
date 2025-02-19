local want = import "@want";

local virtiofs(root, writeable) =
    {root: root, "writeable": writeable};

local output_virtiofs(fsid, q) = 
    {"virtiofs": {"id": fsid, "query": q}};

local amd64_microvm(cores, memory, kernel, kargs, initrd=null, virtiofs={}, output=null) =
    local config = want.blob(std.manifestJsonEx({
        "cores": cores,
        "memory": memory,
        "kernel_args": kargs,
        "virtiofs": virtiofs,
        "output": output,
    }, ""));
    local virtiofsTree = want.pass(
        std.map(function(k) want.input(k, virtiofs[k].root), std.objectFields(virtiofs))
    );
    want.compute("qemu.amd64_microvm", std.flattenArrays([
        [want.input("virtiofs", virtiofsTree)],
        [want.input("kernel", kernel)],
        if initrd != null then [want.input("initrd", initrd)] else [],
        [want.input("vm.json", config)],
    ]));

{
    virtiofs :: virtiofs,
    output_virtiofs :: output_virtiofs,
    amd64_microvm :: amd64_microvm, 
}