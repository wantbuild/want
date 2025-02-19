local want = import "@want";

local wasip1(memory, wasm, inp, args=[], env={}) = 
    local config = want.blob(std.manifestJsonEx({
        args: args,
        env: env,
        memory: memory,
    }, ""));
    want.compute("wasm.wasip1", [
        want.input("program", wasm),
        want.input("input", inp),
        want.input("config.json", config)
    ]);

local nativeGLFS(memory, wasm, inp, args=[], env={}) =
    local config = want.blob(std.manifestJsonEx({
        args: args,
        env: env,
        memory: memory,
    }, ""));
    want.compute("wasm.nativeGLFS", [
        want.input("program", wasm),
        want.input("input", inp),
        want.input("config.json", config)
    ]);

{
    wasip1 :: wasip1,
    nativeGLFS :: nativeGLFS,
}