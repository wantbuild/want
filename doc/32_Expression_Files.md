# Expression Files
Expression files are written as Jsonnet.
They must evaluate to a single Want Expression.

Expression files evaluate to a filesystem object in the output tree.  A file `myexpr.want` in the build input tree, would be replaced at the same path in the build output tree, but the contents would be the evaluation of the expression.

It is impossible for expression files to produce conflicts with other expression files.
This is because each expression file already coexists with other expression files in the source tree, and anything produced from it in the derived tree will be within the same path.

Jsonnet expressions can access a built-in standard library of functions by importing `"want"`.

### Example 1: Download the Alpine minirootfs filesystem**
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

Try running `want ls myexpr.want` to build the output tree for this Expression and list its contents.

You can run `want build` to build the whole module.


