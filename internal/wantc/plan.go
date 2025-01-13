package wantc

import (
	"fmt"

	"github.com/blobcache/glfs"

	"wantbuild.io/want/internal/stringsets"
	"wantbuild.io/want/internal/wantdag"
	"wantbuild.io/want/lib/wantcfg"
)

// Target is a (Set[string], Expr) pair.
type Target struct {
	To        wantcfg.PathSet `json:"to"`
	From      wantdag.NodeID  `json:"from"`
	DefinedIn string          `json:"defined_in"`
	IsExport  bool            `json:"is_export"`
}

func (t Target) Prefix() string {
	return stringsets.BoundingPrefix(SetFromQuery("", t.To))
}

func (t Target) String() string {
	return fmt.Sprintf("%v = %v", t.To, t.From)
}

type Plan struct {
	VFS       *VFS           `json:"-"`
	Graph     glfs.Ref       `json:"graph"`
	Targets   []Target       `json:"targets"`
	Root      wantdag.NodeID `json:"root"`
	NodeCount uint64         `json:"node_count"`
}

func (pl *Plan) Blame(p string) (ret []VFSEntry) {
	ks := stringsets.Prefix(p)
	ret = append(ret, pl.VFS.Get(ks)...)
	return ret
}
