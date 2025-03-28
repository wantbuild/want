
local assertType(ty) = function(x) if std.assertEqual(x.__type__, ty) then x;

// compute evaluates to an operation performed on inputs
local compute(op, inputs) =
    {
        __type__: "expr",
        compute: {
            op: op,
            inputs: std.map(assertType("computeInput"), inputs),
        },
    };

// input prepares an input to a computation.
// to is the path in the input tree.
// from is the value to place in the input tree.
local input(to, from, mode="777") =
    {
        __type__ : "computeInput",
        to: to,
        from: assertType("expr")(from),
        mode: std.parseOctal(mode),
    };

// blob evaluates to a blob literal containing contents
local blob(contents) = {
    __type__: "expr",
    blob: contents,
};

local treeEntry(name, mode, value) = {
    __type__: "treeEntry",
    name: name,
    mode: std.parseOctal(mode),
    value: value,
};

// tree evaluates to a tree literal
local tree(ents=[]) = {
    __type__: "expr",
    tree: std.map(assertType("treeEntry"), ents),
};

// Path Sets

local unit(p) = { __type__: "pathSet", unit: p };
local prefix(p) = { __type__: "pathSet", prefix: p };
local suffix(s) = { __type__: "pathSet", suffix: s };
local union(xs) = { __type__: "pathSet", union: xs};
local not(x) = { __type__: "pathSet", not: x};
local intersect(xs) = { __type__: "pathSet", intersect: xs};
local subtract(l, r) = intersect([l, not(r)]);

local dirPath(p) = if p == "" then prefix("") else union([unit(p), prefix(p + "/")]);

// Select
local select(source, query, pick="") = {
    __type__: "expr",
    selection: {
        source: assertType("source")(source),
        query: assertType("pathSet")(query),
        pick: pick,
    }
};

// selectDir evaluates to the directory at p in the specified source.
// selectDir fails if p is a file.
// if orEmpty is true, then an empty tree will be used when p does not exist.
local selectDir(source, p) = select(source, prefix(p + "/"), pick=p);

// selectFile evaluates to the file at p in the specified source.
// selectFile fails if p is a directory.
local selectFile(source, p) = select(source, unit(p), pick=p);

local metadata() = std.extVar("metadata");

// merge creates a single node from a list of values
local merge(vals) =
    local inputs = std.mapWithIndex(function(i, elem)
        input(to=std.format("%02x", i), from=elem)
    , vals);
    compute("glfs.merge", inputs);

local pass(inputs) = compute("glfs.pass", inputs);

// Statements
local put(dst, src, place="") = {
    __type__: "stmt",
    put: {
        dst: assertType("pathSet")(dst),
        src: assertType("expr")(src),
        place: place,
    },
};

// putFile places the result of src at a single path dst.
local putFile(dst, src) = put(unit(dst), src, place=dst);

// putDir places the result of src at the prefix dst
local putDir(dst, src) = put(prefix(dst), src, place=dst);

// Built-Ins

// importURL imports data from a URL.
// hashAlgo is the algorithm to use for calculating a checksum
// checksum must match or importURL will error
local importURL(url, algo, hash, transforms=[]) =
    local spec = std.manifestJsonEx({
        url: url,
        algo: algo,
        hash: hash,
        transforms: transforms,
    }, "");
    compute("import.fromURL", [
        input(to="", from=blob(spec)),
    ]);

local importGit(url, commitHash, branch="master") =
    local spec = std.manifestJsonEx({
        url: url,
        branch: branch,
        commitHash: commitHash,
    }, "");
    compute("import.fromGit", [
        input(to="", from=blob(spec)),
    ]);

local importGoZip(path, version, hash) =
    local spec = std.manifestJsonEx({
        path: path,
        version: version,
        hash: hash,
    },"");
    compute("import.fromGoZip", [
        input(to="", from=blob(spec))
    ]);

local importOCIImage(name, algo, hash) =
    local spec= std.manifestJsonEx({
        name: name,
        algo: algo,
        hash: hash,
    }, "");
    compute("import.fromOCIImage", [
        input(to="", from=blob(spec))
    ]);

local unpack(x, transforms=[]) =
    local spec = std.manifestJsonEx({
        transforms: transforms
    }, "");
    compute("import.unpack", [
        input("x", x),
        input("config.json", blob(spec)),
    ]);

// pick finds subpath p in tree x
local pick(x, p) = compute("glfs.pick", [
    input(to="x", from=x),
    input(to="path", from=blob(p)),
]);

local place(x, p) = compute("glfs.place", [
    input(to="x", from=x),
    input(to="path", from=blob(p)),
]);

local filter(x, pathSet) =
    compute("glfs.filterPathSet", [
        input(to="x", from=x),
        input(to="filter", from=blob(std.manifestJsonEx(pathSet, ""))),
    ]);

// diff finds the differences between two trees.
// it presents them as left-only, right-only and both
local diff(left, right) = compute("glfs.diff", [
    input(to="left", from=left),
    input(to="right", from=right),
]);

local evalSnippet(snip) = 
    local graph = compute("want.compileSnippet", [input("", snip)]);
    local output = compute("graph.eval", [input("", graph)]);
    compute("graph.pickLastValue", [input("", output)]);

{
    // Literal

    blob :: blob,
    tree :: tree,
    treeEntry :: treeEntry,


    // Path Sets

    unit :: unit,
    prefix :: prefix,
    suffix :: suffix,
    union :: union,
    intersect :: intersect,
    not :: not,
    subtract :: subtract,

    dirPath :: dirPath,

    // Selections

    select :: select,
    selectFile :: selectFile,
    selectDir :: selectDir,

    // Compute

    input :: input,
    compute :: compute,

    // Statements
    put :: put,
    putFile :: putFile,
    putDir :: putDir,

    // GLFS
    pick :: pick,
    place :: place,
    filter :: filter,
    merge :: merge,
    pass :: pass,
    diff :: diff,

    // Metadata
    metadata :: metadata,

    // Import
    importURL :: importURL,
    importGit :: importGit,
    importGoZip :: importGoZip,
    importOCIImage :: importOCIImage,
    unpack :: unpack,

    // Want
    evalSnippet :: evalSnippet, 
}
