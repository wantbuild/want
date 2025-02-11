# Introduction

## What is Want?
Want is a build system.

Build systems are programs that run other programs to compute outputs from inputs, often more efficiently than rerunning the whole computation for every change in the input.

Want is sometimes called a "hermetic" build system, which means that the build computation is cut off from external resources.
Want knows about all the bits going into every computation, and there is no way to sneak different or extra bits into a computation without Want knowing about it.
This allows Want to cache every computation in the build, and avoid repeated work.

## Why would I use Want?
I've you've ever tried to get up and running with a new software project, you will have dealt with the problem of downloading all the tools that the project uses.

Even for simple tasks like building the docs, or producing shippable executables, you may have to install several tools.  This requires developers to know about and understand tools which they will never work with themselves, to merely benefit from the output of a computation that others have set up.

If you or someone else put in the time to get something to work, it should also just work for you going forward.  You shouldn't have to replay previous steps like: picking the right version, learning command line arguments, order of operations, etc.

With Want, every build target you add will be computed in exactly the same way on your laptop tomorrow, on your co-workers development machine, and in CI.  Want forces build steps to be fully and exactly specified, so they can be reproduced by anyone at anytime, in any environment.

## Why is it called Want?
You write down what you want, and Want builds it.

More seriously: `.want` and `.wants` were good extensions for the files where you specify build targets, and naming the entire build system after them came later.
