# Blobs, Trees, and Refs

Want stores all data using a format called the *Git-Like Filesystem* (GLFS).
GLFS is not identical or even a superset of the formats used by [Git](https://git-scm.com/) the version control software.
However, it is very similar in terms of the shape of the datastructure.

GLFS is an open source project, distinct from Want, which you can explore [here](https://github.com/blobcache/glfs)

The first step in a build is to import the entire root module from the local filesystem into Want where it is represented as a GLFS filesystem.

## Blobs
A Blob is an arbitrarily large sequence of bytes.
It supports random access, so you can read from anywhere in the sequence efficiently.
Blobs are immutable, changing the bytes of a Blob would produce a different Blob.

Terminal nodes in a [POSIX](https://en.wikipedia.org/wiki/POSIX) Filesystem (regular files and symbolic links) are represented as Blobs in GLFS.
A Blob does not contain any references to any other filesystem objects.

## Refs
*Ref* stands for *Reference*.
It is information which can be used to retrieve a filesystem object, but is typically much smaller than the object itself.
A *Ref* is like an IOU for an immutable filesystem snapshot.

Refs contain a cryptographic hash of the content that they point to.
They also contain the Type of the data that they point to.  The Type is either `tree` or `blob`.
Refs to Trees and Refs to blobs contain an equivalent amount of information.  Phrased differently, a Ref to a Tree will not take up more or less space than a Ref to a Blob.

Throughout Want, Refs are used extensively.
All filesystem data is passed around as Refs.
Expressions in the build language evaluate to Refs in the output.

## Trees
A Tree is the GLFS analog to a POSIX filesystem directory.
A *Tree* is a set of entries, where each entry contains:
- Name
- Mode
- *Ref*

Names cannot contain the character `/`, and must be unique across all the other entires in a tree.
Mode contains permission bits; they are mostly needed for compatibility when converting back to a POSIX filesystem.
The *Ref* refers to another object in the filesystem, as was discussed above.

Trees form a recursive datastructure because they can contain references to other Trees.
Trees cannot contain a reference to themselves or transitively contain trees with a reference to themselves.
This is enforced by the cryptographic hash used in each *Ref*.

