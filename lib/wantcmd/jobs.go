package wantcmd

import (
	"go.brendoncarroll.net/star"
)

var jobCmd = star.NewDir(star.Metadata{
	Short: "inspect and manage jobs",
}, map[star.Symbol]star.Command{})
