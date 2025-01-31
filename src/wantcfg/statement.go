package wantcfg

type Statement struct {
	Put    *Put    `json:"put,omitempty"`
	Export *Export `json:"export,omitempty"`
}

func (s Statement) Expr() Expr {
	switch {
	case s.Put != nil:
		return s.Put.Src
	case s.Export != nil:
		return s.Export.Src
	default:
		panic("empty statement")
	}
}

type Put struct {
	Dst PathSet `json:"dst"`
	Src Expr    `json:"src"`
	// TODO: Add Place here instead of inferring bounding prefix?
}

type Export struct {
	Dst PathSet `json:"dst"`
	Src Expr    `json:"src"`
}
