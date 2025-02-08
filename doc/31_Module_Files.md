# Module Files
Want operates on units called Modules.
A Module is a Tree, with a Blob containing Jsonnet at the path `WANT`.

The `WANT` file provides module level configuration.
It also serves as a sanity check that the directory is supposed to be built by Want.
The structure and contents of this configuration file are discussed in this section.

> No user configuration is required by default.
The easiest way to create a Want module is to run `init`.

## Jsonnet Context 
Want modules have access to a single import `@want`, which contains the standard library.
It is imported in the default generated `WANT` file.  These functions are more than enough to get any libaries needed to plan the build.

## Allowed Keys

### `ignore: PathSet`
This controls which paths will be ignored by Want.  Anything in this set will not even be imported.

By default paths used by `.git` are ignored.
If your project does not use Git, this configuration can be removed.  If your project does use Git, changing this can significantly slow down importing the module.

### `namespace: Map[String, Expr]`
`namespace` should resolve to a JSON object where the values are filesystem Expressions.

By default `namespace` contains a single entry for `want`.
```jsonnet
{
    namespace: {
        "want": {blob: importstr "@want"},
    }
}
```

This allows the standard library to be used anywhere in the module.  Anything in this namespace object will be accessible for import in any `.want` or `.wants` file in the module.

> Dependencies added here should be for planning the build.  You might have a single dependency here per programming language in your project. Most of the build dependencies for a project should be in the other configuration files.