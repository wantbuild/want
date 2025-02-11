# Concepts

This section provides a conceptual overview of Want.
If something doesn't make sense or you want to learn more about Want's model, this is a good section to read.

## TLDR
> Want = Git + Compute

Git models filesystem data as immutable Trees and Blobs.
Directories are Trees, and files are Blobs.  In Want, all Values are Trees or Blobs.

As you will learn in the later sections: you can specify them literally, you can select them from the project source, you can get them from the network, and you can compute them.

It doesn't matter where they come from, all data turns into Trees and Blobs in the end.

Want allows you to perform operations (which are Trees & Blobs) on inputs (which are Trees & Blobs) to produce outputs (which are also Trees & Blobs).

There are more details, but if you know how Git works, then just imagine using Git trees to define programs, and their inputs.  The programs output trees as well.