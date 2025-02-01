package linux

import (
	_ "embed"
	"slices"

	// prevent any packages in from depending on this
	_ "wantbuild.io/want/src/want"
)

//go:embed bzImage
var bzImage []byte

func BzImage() []byte {
	return slices.Clone(bzImage)
}
