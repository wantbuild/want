# Statement Files
Statement files are any files ending in `.wants` (the `s` stands for statement)

Statement files are also Jsonnet, but instead of expressing a JSON object, they must express a JSON list of statements.

At the time of writing there is one kind of statement: `put`.

## Put Statement
Put statements write an expression to another location in the build output.

e.g.
```jsonnet
local want = import "@want";

[
    want.put(want.unit("./my-statement-output.txt"), want.blob("foo"))
]
```

You can have as many statement files, and as many statements per file, as you *want*.
