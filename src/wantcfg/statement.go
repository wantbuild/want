package wantcfg

type Statement struct {
	Put *Put `json:"put,omitempty"`
}

func (s Statement) Expr() Expr {
	switch {
	case s.Put != nil:
		return s.Put.Src
	default:
		panic("empty statement")
	}
}

type Put struct {
	// Dst is the PathSet this statement occupies within the module.
	// Only these paths will be taken from Src.
	Dst PathSet `json:"dst"`
	// Src should evaluate to an expression for the root of the module.
	Src Expr `json:"src"`
	// Place will wrap Src in place operation for the given path
	Place string `json:"place,omitempty"`
}
