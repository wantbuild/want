# Using Want

This section will get you up and running with Want.

> Most of the time you should run `want build`.
This command is suitable to use as a pre-merge check.
It will build all the targets in the current module.
It does not and can not perform publishing or other side effects.

## Creating a Want Module
To start using Want with a new project, first we need to initialize a Module.
Change the current working directory to the root of your project source code, and run the following command.

```shell
$ want init
```

You should see a file called `WANT` in the current directory.
This file contains configuration for Want in the [Jsonnet](https://jsonnet.org/) configuration language.
Jsonnet is used for all the configuration files in Want.

## Creating a Build Target
Want calls the things that you, the user *want*, but don't yet have, and need to build *Targets*.  There are 2 ways to specify targets: as Expressions, and as Statements.

They are each good for different things; in this example we will use an expression.

Create a file `myexpr.want` anywhere in your project, and fill it with the following
```jsonnet
local want = import "@want";

want.blob("Hello World!\n")
```

Now run

```shell
$ want cat myexpr.want
Hello World!

```

## Inspecting the Build Output

Normally `cat myexpr.want` would print the Jsonnet file above, but instead it printed just the "Hello World!" part.  This is because `want cat` reads from the build *output*, while the regular `cat` reads from the regular filesystem, which is the build *input*.

Want organizes build targets into an output filesystem.
This filesystem only exists virtually, within want, but it can be inspected using similar tools to UNIX (`ls` and `cat`).

`want ls <path>` will list the contents of a tree in the build output.  Compare this to the output of regular `ls`.

This simple translation of inputs to outputs is what makes Want so easy to use and reason about.  You will never find yourself in a situation where you don't know when/how/where a Target is being computed.

Armed with just `want ls`, `want cat`, and `want build`, you can get pretty far.  To learn more about what you can build with Want, read the [Build Configuration](./30_Build_Configuration.md) section.