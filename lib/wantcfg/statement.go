package wantcfg

type Statement struct {
	Put    *Put    `json:"put,omitempty"`
	Export *Export `json:"export,omitempty"`
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
