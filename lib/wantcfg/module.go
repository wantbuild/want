package wantcfg

// ModuleConfig contains project level configuration
// which should be committed in version control and is suitable for
// every contributor to the project
type ModuleConfig struct {
	Name   string  `json:"name"`
	Ignore PathSet `json:"ignore"`
}

func DefaultProjectConfig(name string) *ModuleConfig {
	return &ModuleConfig{
		Name:   name,
		Ignore: DirPath(".git"),
	}
}
