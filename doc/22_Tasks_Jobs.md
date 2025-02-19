# Tasks & Jobs

## Tasks
A *Task* is a well defined unit of work.
Well defined means that all the information that could be required to complete the Task is explicitly contianed or precisely implied by the Task.

*Tasks* have 2 parts: an *Operation* and an *Input*.
*Operation* refers to one of a small number of built in operators.
Here are some example *Operations*
- `glfs.place`
- `glfs.pick`
- `import.fromURL`
- `wasm.wasip1`
- `qemu.amd64_microvm`

> This list is not exhausted, but there are only around a dozen of them.

*Input* is small payload of bytes.
In practice this is a [Ref](./21_Blobs_Trees_Refs.md#refs), which references a filesystem to perform the operation on.

These 2 components are included in a hash which becomes the *TaskID*.

The *TaskID* is used as key for looking up the result of Task.
Want caches the output of Tasks when they are successfully completed, and checks this cache before computing a *Task*.

An immediate rerun of `want build` without any changes will result in all the same *Tasks*, which can all be skipped.

## Jobs
*Jobs* are used to track running computations in Want.

*Jobs* are created to compute *Tasks*.
*Jobs* contain active state like whether they have completed and the start and end time.

*Jobs* are organized into a forest of hierarchical trees.  All the major operations in Want create a new tree in the forrest, which is a "root" *Job*.
That job may spawn additional child *Jobs*.

Child *Jobs* give computations access to additional compute, since the Child *Jobs* are computed in parallel.
Child *Jobs* also present a way for *Jobs* to utilize the cache, which further benefits performance.

*Jobs* are unable to spawn root or sibling *Jobs*, they can only spawn *Jobs* which are their immediate children.
This structuring prevents runaway computations.
Every new *Job* has an audit trail, which ultimately goes back to the User asking for something to be done.
