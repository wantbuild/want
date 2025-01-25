local want = import "want";

local bzImage = want.selectFile(want.FACT, "./linux/bzImage");

local dumbInit = want.importURL(
    url="https://github.com/Yelp/dumb-init/releases/download/v1.2.5/dumb-init_1.2.5_x86_64",
    algo="SHA256",
    hash="e874b55f3279ca41415d290c512a7ba9d08f98041b28ae7c2acb19a545f1c4df"
);

{
    bzImage :: bzImage,
    dumbInit :: dumbInit,
}