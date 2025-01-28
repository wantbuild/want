# Build Configuration

Want configuration is mostly done using 2 types of files: *Expression* files, which end in `.want` and *Statement* files, which end in `.wants`.

The 3rd configuration file is the module level config, which you can read about in the next section.

## Module Config
Want operates on units called Modules.
A Module is a Tree, with a Blob containing Jsonnet at the path `WANT`.
This `WANT` file, provides module level configuration.
It also serves as a sanity check that the directory is supposed to be built by Want.

No user configuration is required by default.
The easiest way to create a Want module is to run `init`.
```shell
$ want init
$ ls
WANT
```

Then you can edit it to your liking.

## Expression Files
Expression files are written as Jsonnet.  They evaluate to a JSON structure which is then used to create a build graph.

Expression files evaluate to a filesystem object in the output tree.  A file `myexpr.want` in the build input tree, would be replaced at the same path in the build output tree, but the contents would be the evaluation of the expression.

It is impossible for expression files to produce conflicts with other expression files.
This is because each expression file already coexists with other expression files in the source tree, and anything produced from it in the derived tree will be within the same path.

Jsonnet expressions can access a built-in standard library of functions by importing `"want"`.

### Example 1: Download the Alpine root filesystem**
In a file called `myexpr.want`
```jsonnet
local want = "want";

want.importURL(
    url="http://dl-cdn.alpinelinux.org/alpine/latest-stable/releases/x86_64/alpine-minirootfs-3.21.2-x86_64",
    algo="SHA256",
    hash="4aa3bd4a7ef994402f1da0f728abc003737c33411ff31d5da2ab2c3399ccbc5f",
    transforms=["ungzip", "untar"])
```

This would be replaced in the output with the alpine minirootfs
```shell 
$ want ls myexpr.want
755 tree gsPPzRn4RplJ-A1y bin
755 tree flC8iUMtcPPVF3re dev
755 tree ypQygCpJM8hd2GLF etc
755 tree flC8iUMtcPPVF3re home
755 tree GmEGuDFDQW3dR5wy lib
755 tree HVW60D2PJ_3_amD6 media
755 tree flC8iUMtcPPVF3re mnt
755 tree flC8iUMtcPPVF3re opt
555 tree flC8iUMtcPPVF3re proc
700 tree flC8iUMtcPPVF3re root
755 tree flC8iUMtcPPVF3re run
755 tree RtfdNc5JvrBwxphU sbin
755 tree flC8iUMtcPPVF3re srv
755 tree flC8iUMtcPPVF3re sys
777 tree flC8iUMtcPPVF3re tmp
755 tree 7ygjwYBFIH_ckNLl usr
755 tree 3eskP3ejV1fdXjV6 var
$ 
```

You can also run `want build` to build the whole module.
```shell
$ want build

```

More about the standard library in [Jsonnet Library](./21_Jsonnet_Library.md).

## Statement Files
Statement files are any files ending in `.wants` (the `s` stands for statement)

Statement files are also Jsonnet, but instead of expressing a JSON object, they must express a JSON list of statements.
There are two statements:
- `put`
- `export`

Put statements write an expression to another location in the VFS.
Exports are like Puts, but additionally write data to the real Filesystem provided by the operating system.

Export statements can be useful for generating files that you want a local program like an IDE to see.
It's good practice to exclude any files that you export from version control.
This is done with a `.gitignore` file in Git.

You can have as many statement files, and as many statements per file, as you *want*.

Exports are run after the build succeeds, not during.

## Build Virtual Filesystem  (VFS)
Each build in Want is centered around creating a Filesystem tree.
That tree is never written out to the real filesystem, because that would clutter it with build artifacts and intermediary products.
You can write out portions of it with `export` statements.
