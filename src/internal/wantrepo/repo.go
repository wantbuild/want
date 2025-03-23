package wantrepo

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"blobcache.io/glfs"
	"go.brendoncarroll.net/state/cadata"
	"wantbuild.io/want/src/internal/glfsport"
	"wantbuild.io/want/src/internal/stringsets"
	"wantbuild.io/want/src/internal/wantc"
	"wantbuild.io/want/src/wantcfg"
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
	modCfg, err := wantc.ParseModuleConfig(cfgData)
	if err != nil {
		return nil, err
	}
	ignoreSet := wantc.SetFromQuery("", modCfg.Ignore)
	return &Repo{
		dir:       p,
		rawConfig: string(cfgData),
		config:    *modCfg,
		ignoreSet: ignoreSet,
	}, nil
}

func Init(workDir string) error {
	cfgPath := filepath.Join(workDir, "WANT")
	cfgFile, err := os.OpenFile(cfgPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer cfgFile.Close()
	cfgStr := defaultModuleConfig()
	if _, err := cfgFile.Write([]byte(cfgStr)); err != nil {
		return err
	}
	return cfgFile.Sync()
}

type Repo struct {
	dir string

	rawConfig string
	config    wantcfg.ModuleConfig
	ignoreSet stringsets.Set
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
	return !r.ignoreSet.Contains(x)
}

func (r *Repo) Metadata() map[string]any {
	return map[string]any{}
}

// Import imports a filesystem from the Repo
func (repo *Repo) Import(ctx context.Context, dst cadata.PostExister, p string) (*glfs.Ref, error) {
	imp := glfsport.Importer{
		Store:  dst,
		Dir:    repo.RootPath(),
		Filter: repo.PathFilter,
		Cache:  &glfsport.MemCache{},
	}
	return imp.Import(ctx, p)
}

// Export writes a filesystem to (part of) the Repo
func (r *Repo) Export(ctx context.Context, src cadata.Getter, p string, ref glfs.Ref) error {
	exp := glfsport.Exporter{
		Cache:   glfsport.NullCache{},
		Store:   src,
		Dir:     r.dir,
		Clobber: true,
	}
	return exp.Export(ctx, ref, p)
}

func defaultModuleConfig() string {
	return `local want = import "@want";
{
	ignore: want.dirPath(".git"),
	namespace: {
		want: want.blob(importstr "@want"),
	}
}
`
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
	finfo, err := os.Stat(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return !finfo.IsDir(), nil
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
