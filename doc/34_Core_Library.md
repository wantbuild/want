# Want Core Library

The Want Core library is available in Module files (`WANT`) as `@want`.

e.g.
```jsonnet
local want = import "@want";
```

The auto-generated module file includes the standard library in the module namespace as `@want` as well, although this is configurable.

## Literals
These functions specify an output directly, without any further computation.

### `blob(contents: String): Expr`
Evaluates to a Blob literal.

### `treeEntry(name: String, mode: String, e Expr): TreeEntry`
Specifies an entry in a Tree.
- `name` is the name within the tree.
- `mode` is parsed as an octal number.
- `e` is the contents of the entry.

This is just UNIX permission bits attached to a Filesystem Expr.  It is not a valid Filesystem expression on its own.

### `tree(ents: List[TreeEntry]): Expr`
Evaluates to a Tree literal.

> NOTE: All elements of a tree must be known.  To assemble a tree during the build see `pass`

## Path Sets
These functions specify sets of Paths, which are used in several places in the API.

### `unit(p: String): PathSet`
A set with 1 path `p` in it.

### `prefix(p: String): PathSet`
All paths that start with `p`.

### `suffix(s: String): PathSet`
All paths that end with `p`.

### `not(x: PathSet): PathSet`
All the paths that are not in x

### `intersect(xs: List[PathSet]): PathSet`
The paths in common between all the sets in `xs`. i.e in `xs[0]` and `xs[1]` and `xs[2]`, etc...

### `union(xs: List[PathSet]): PathSet`
The paths which are in any of the sets in `xs`. i.e. in `xs[0]` or `xs[1]` or `xs[2]`, etc...

### `subtract(l: PathSet, r: PathSet): PathSet`
A convenience function which is equivalent to `intersect([l, not(r)])`.
- `l` is the left side of a subtraction expression `l - r`
- `r` is the right side of an subtraction expression `l - r`

## Selections
Selections are Exprs which refer to build inputs or outputs.
There are two sources `GROUND` and `DERIVED` which are constants available in every jsonnet context.  They are **not** imported from the standard library.

*Correct*
```jsonnet
local want = import "@want";

select(GROUND, want.prefix(""))
```

*Incorrect*
```jsonnet
local want = import "@want";

select(want.GROUND, want.prefix(""))
```

Selections have the potential to produce cycles because it is possible to express a circular selection.
Want will return an error quickly when a cycle is detected.
There is no risk of launching an infinite circular process.

Selecting from `GROUND` never produces a cycle.  `GROUND` is the state of Module as-is, before any build steps have been computed.

Selecting from `DERIVED` can produce cycles, because it depends on the build output.
An expression which makes a selection can only be computed after expressions which output to that selection.


### `select(src: Source, q: PathSet): Expr`
Evaluates to a filesystem containing paths in q, with data from src.

### `selectFile(src: Source, p: String): Expr`
This is a convenience function for selecting files.
It is equivalent to selecting `unit(p)` and then calling `pick` to extract the file.

### `selectDir(src: Source, p: String): Expr`
This is a convenience function for selecting directories.
It is equivalent to selecting `union([unit(p), prefix(p + "/")])`. and then calling `pick` to extract the directory.

## Compute
These functions allow computations to be specified

### `input(name: String, expr: Expr): Input`
Specifies an input to a computation.
This is not a valid Filesystem expression on it's own.

### `compute(op: String, inputs: List[Inputs]): Expr`
Evaluates to a computed Filesystem.
An operation identified by `op` will be performed on the inputs provided.
The inputs will also be computed if the have not been already.

These are the core functions in Want that everything is based on.

### `pass(inputs: List[Inputs]): Expr`
Short for "passthrough".
This performs no additional computation other than assembling the inputs into a single directory, which is done for all computations.

## Git-Like Filesystem
Want represents all data in a format called the *Git-Like Filesystem* or *GLFS* for short.  Primitive operations on the GLFS Refs are essential.

### `place(x: Expr, p: String): Expr`
Creates a chain of Trees according to the path `p`.  They will lead to the value of x.

For example if p was `a/b/c/d` then the resulting filesystem would contain:
```
a/b/c/d => x
```

### `pick(x: Expr, p: String): Expr`

For example if x contained
```
a.txt       => 0000
b/foo.txt   => 1111
c/d/e.txt   => 2222
f.txt       => 3333
```

Then `pick(x, "b/foo.txt")` would evaluate to `1111`

### `filter(x: Expr, query: PathSet): Expr`
Returns `x` but only containing paths in `query`.


## Imports
Computations in Want are cut off from the network and other external resources.
The only way to retrieve information from the external resources is through the import system.

### `importURL(url: String, algo: String, hash: String, transforms: []String): Expr`
Downloads data from the url.
The data will be hashed using the specified algorithm.
If the actual hash does not match `hash`, then the import will fail.
Transforms are applied after the hash check.

**Hash Algorithms**
- `SHA2-256`, `SHA256` 
- `SHA2-512`, `SHA512` 
- `SHA3-256`
- `BLAKE2b-256`
- `BLAKE3-256`

**Transforms**
- `ungzip`
- `unzstd`
- `unxz`
- `untar` (must be last)
- `unzip` (must be last)

### `importGit(repoUrl: String, commitHash: String): Expr`
Imports the Git Tree from the Commit identified by `commitHash`.

### `importOCI(url: String, algo: String, hash: String): Expr`
Imports an [Open Container Initiative](https://opencontainers.org/) ([Docker](https://www.docker.com)) Image.

e.g.
```jsonnet
want.importOCIImage(
    "docker.io/library/alpine",
    algo="sha256",
    hash="48d9183eb12a05c99bcc0bf44a003607b8e941e1d4f41f9ad12bdcc4b5672f86",
)
```

### `unpack(x: Expr, transforms: []String): String`
Unpack isn't a real import, it doesn't use the network, but it uses the transform functionality from the import system.
It takes an existing filesystem expression and applies the transforms in order to return the output.

`unpack` supports the same transforms as `importURL`

## Statements
Statements can only be used in a statement file (ending in `.wants`)

### `put(dst: PathSet, x: Expr): Stmt`
Creates a target occupying `dst` in the build output.
The contents of dst will be taken by copying the paths from the evaluation of `x`.

### `putFile(dst: String, x: Expr): Stmt`
Creates a target, which will be a single path `dst` in the build output.
It will be populated by the evaluation of `x`, which must evaluate to a file.
It is equivalent to `put(unit(dst), place(x, dst))`

### `putDir(dst: String, x: Expr): Stmt`
Creates a target occupying `prefix(dst)` in the build output.
It will be populated by the evaluation of `x`.
It is equivalent to `put(union([unit(dst), prefix(dst + "/")]), place(x, dst)`.

