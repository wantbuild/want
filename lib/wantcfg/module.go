package wantcfg

// ModuleConfig contains project level configuration
// which should be committed in version control and is suitable for
// every contributor to the project
type ModuleConfig struct {
	Ignore PathSet `json:"ignore"`
}

func DefaultProjectConfig() *ModuleConfig {
	return &ModuleConfig{
		Ignore: DirPath(".git"),
	}
}
