# Want Standard Library

## Core
These are the core functions on which everything else is built.

### `blob(contents: String): Expr`
Evaluates to a Blob literal.

### `treeEntry(mode: String, e Expr): TreeEntry`
Specifies an entry in a Tree.
This is just UNIX permission bits attached to a Filesystem Expr.  It is not a valid Filesystem expression on its own.

### `tree(ents: Map[String, TreeEntry]): Expr`
Evaluates to a Tree literal.

> NOTE: All elements of a tree must be known.  To assemble a tree see `pass`

### `input(name: String, expr: Expr): Input`
Specifies an input to a computation.
This is not a valid Filesystem expression on it's own.

### `compute(op: String, inputs: List[Inputs]): Expr`
Evaluates to a computed Filesystem.
An operation identified by `op` will be performed on the inputs provided.
The inputs will also be computed if the have not been already.

These are the core functions in Want that everything is based on.

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

Then `pick(x, "b/foo.txt")` would evaluate to 1111

### `filter(x: Expr, query: PathSet): Expr`
Returns `x` but only containing paths in `query`.