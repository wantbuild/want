package stores

import (
	"blobcache.io/blobcache/src/bcsdk"
	"blobcache.io/blobcache/src/blobcache"
	"blobcache.io/blobcache/src/schema"
	"lukechampine.com/blake3"
)

const MaxBlobSize = 1 << 21

func Hash(x []byte) blobcache.CID {
	return blake3.Sum256(x)
}

func NewMem() bcsdk.RWD {
	return schema.NewMem(Hash, MaxBlobSize)
}

func NewVoid() bcsdk.RO {
	return Union{}
}
