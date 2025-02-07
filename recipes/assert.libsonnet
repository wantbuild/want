local want = import "@want";

local subsetOf(x, superset) =
    want.compute("assert.all", [
        want.input("x", x),
        want.input("subsetOf", superset),
    ]);

local pathExists(x, path) =
    want.compute("assert.all", [
        want.input("x", x),
        want.input("pathExists", want.blob(path)),
    ]);

{
    pathExists :: pathExists,
    subsetOf :: subsetOf,
}