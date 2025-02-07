local want = import "@want";

local importImage(name, hash, algo="sha256") =
    want.importOCIImage(name, algo, hash);

{
    importImage :: importImage,
}