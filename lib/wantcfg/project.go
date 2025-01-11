package wantcfg

// ProjectConfig contains project level configuration
// which should be committed in version control and is suitable for
// every contributor to the project
type ProjectConfig struct {
	Name   string  `json:"name"`
	Ignore PathSet `json:"ignore"`
}

func DefaultProjectConfig(name string) *ProjectConfig {
	return &ProjectConfig{
		Name:   name,
		Ignore: DirPath(".git"),
	}
}
