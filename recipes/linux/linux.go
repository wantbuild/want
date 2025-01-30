package linux

import (
	_ "embed"

	// prevent any packages in from depending on this
	_ "wantbuild.io/want/src/want"
)

//go:embed bzImage
var BzImage []byte
