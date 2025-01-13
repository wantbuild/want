package stores

import (
	"go.brendoncarroll.net/state/cadata"
	"lukechampine.com/blake3"
)

const MaxBlobSize = 1 << 21

func Hash(x []byte) cadata.ID {
	return blake3.Sum256(x)
}

func NewMem() cadata.Store {
	return cadata.NewMem(Hash, MaxBlobSize)
}

func NewVoid() cadata.Getter {
	return Union{}
}
