package wantrepo

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"wantbuild.io/want/lib/wantcfg"
)

func Open(p string) (*Repo, error) {
	p, err := filepath.Abs(p)
	if err != nil {
		return nil, err
	}
	repoConfigPath := filepath.Join(p, "WANT")
	cfgFile, err := os.Open(repoConfigPath)
	if err != nil {
		return nil, err
	}
	defer cfgFile.Close()
	cfgData, err := io.ReadAll(cfgFile)
	if err != nil {
		return nil, err
	}
	return &Repo{
		dir:       p,
		rawConfig: string(cfgData),
	}, nil
}

func Init(workDir string, projectName string) error {
	cfgPath := filepath.Join(workDir, "WANT")
	cfgFile, err := os.OpenFile(cfgPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer cfgFile.Close()
	cfg := wantcfg.DefaultProjectConfig(projectName)
	data := jsonMarshalPretty(cfg)
	if _, err := cfgFile.Write(data); err != nil {
		return err
	}
	return cfgFile.Sync()
}

type Repo struct {
	dir string

	rawConfig string
	config    wantcfg.ModuleConfig
}

func (r *Repo) RawConfig() string {
	return r.rawConfig
}

func (r *Repo) Config() wantcfg.ModuleConfig {
	return r.config
}

func (r *Repo) RootPath() string {
	return r.dir
}

func (r *Repo) PathFilter(x string) bool {
	return x != ".git" && !strings.HasPrefix(x, ".git/")
}

func (r *Repo) Metadata() map[string]any {
	return map[string]any{}
}

// IsRepo returns (true, nil) if the directory contains a want repo
func IsRepo(p string) (bool, error) {
	info, err := os.Stat(p)
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, nil
	}
	cfgPath := filepath.Join(p, "WANT")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if err := json.Unmarshal(data, &struct{}{}); err != nil {
		return false, nil
	}
	return true, nil
}

func FindRepo(p string) (bool, string, error) {
	for {
		yes, err := IsRepo(p)
		if err != nil {
			return false, "", err
		}
		if yes {
			return true, p, nil
		}
		p2 := filepath.Dir(p)
		if p2 == p {
			return false, "", nil
		}
		p = p2
	}
}

func jsonMarshalPretty(x any) []byte {
	data, err := json.MarshalIndent(x, "", "  ")
	if err != nil {
		panic(err)
	}
	return data
}
