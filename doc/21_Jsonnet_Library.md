# Jsonnet Library

Within a Want expression or statement file two libraries are available for import.
- `want`, typically imported as `local want = import "want"`
- `wants`, typically imported as `local wants = import "wants";`

Other files can be imported using the path.
Paths starting with a `./` are relative to the file's directory.
Paths starting with an alphanumeric are interpretted as starting at the project root (where the `WANT` file is defined).

Most of the utilities are in the `want` library, which deals with expressions.
Statements can be created using the `wants` library.

## Literals
A few functions are provided for specifying data literally to Want.

### `blob(contents)`
Evaluates to a Blob filesystem object containing contents.

### `emptyDir()`
Evaluates to an empty directory

## Selections
Functions in this section select portions of the build tree.

Multiple sources are available to select from.
- `DERIVED` the current build tree, including the output of other expression/statements in the current build.
You might think that this should be the only option (and it is the default), but it makes it possible to create circular dependencies, which are not allowed.
So sometimes it's not the right source.
- `FACT` the source before any build steps have happened.
An expression can't create a circular dependency if it only selects from `FACT`.

## `selectFile(source, selector)`
Selects a file from the build VFS or the fact data.
If the selection is not a file, then the build will fail.

## `selectDir(souce, selector)`
Selects a directory from the build VFS or the fact data.
If the selection is not a directory, then the build will fail.
If orEmpty is true, then an empty selection is replaced with an empty tree.

## Computation
This is where things get interesting.
Want wouldn't be much of a build system if you couldn't build anything.

### input(to, from)
- `to` is the path in the input tree to place from
- `from` is some other filesystem expression.

### `compute(op, inputs)`
Evaluates to the result of performing op on the inputs.


## Importing
Often dependencies have to be retrieved over the network.
This creates an obvious problem since the network introduces non-determinism.
To create a deterministic output, all imports from the network must match a provided hash.
The build step will either fail, or the checksum will match.

> If you aren't sure what the hash is, you can leave it empty and Want will print it in the error message.  This is a good way to implement the "trust on first use" pattern.

### `importURL(url, algo, hash, transforms=[])`
Imports data from a URL over the network.

- `url` The url to import.  Supported schemas are `http` and `https`.
- `algo` is the name of the hash algorithm to use when calculating the hash.
The following hash algorithms are supported
    - `SHA1`
    - `SHA256`
    - `SHA512`
    - `SHA3-256`
    - `BLAKE2b-256`
    - `BLAKE2s-256`
    - `BLAKE3-256`
- `hash` is an encoded checksum to check the download against.
The encoding will be automatically figured out based on the length and the hash algorithm.
Currently: Base64, and Hex are both supported.
- `transforms` are transforms to apply to the data.
The following transforms are supported.
They can be chained together to e.g. import a gzip'd TAR archive.
    - `unxz`
    - `ungzip`
    - `unzstd`
    - `untar`
    - `unzip`

### `importGit(url, hash, branch="master")`
Imports data from a git repository over the network.

- `url` The url to import.
- `hash` The commit hash.
- `branch` The branch to look for the hash in.

