package wantcmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	"go.brendoncarroll.net/star"

	"wantbuild.io/want/internal/wantdb"
	"wantbuild.io/want/lib/wantrepo"
)

// Root
func Root() star.Command {
	return rootCmd
}

var rootCmd = star.NewDir(star.Metadata{Short: "want build system"},
	map[star.Symbol]star.Command{
		"init":   initCmd,
		"status": statusCmd,
		"import": importCmd,

		"blame": blameCmd,

		"build": buildCmd,
		"ls":    lsCmd,
		"cat":   catCmd,

		"job": jobCmd,
	},
)

var initCmd = star.Command{
	Metadata: star.Metadata{Short: "initialize want in the current directory"},
	Pos:      []star.IParam{projNameParam},
	F: func(c star.Context) error {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		return wantrepo.Init(wd, projNameParam.Load(c))
	},
}

var statusCmd = star.Command{
	Metadata: star.Metadata{Short: "print status information"},
	Flags:    []star.IParam{dbParam},
	F: func(c star.Context) error {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		yes, repoRoot, err := wantrepo.FindRepo(wd)
		if err != nil {
			return err
		}
		if yes {
			repo, err := wantrepo.Open(repoRoot)
			if err != nil {
				return err
			}
			c.Printf("ROOT: %s\n", repoRoot)
			c.Printf("CONFIG: %v\n", repo.RawConfig())
		} else {
			c.Printf("%s is not in a want project\n", wd)
		}
		return nil
	},
}

var dbParam = star.Param[*sqlx.DB]{
	Name:    "db",
	Default: star.Ptr(""),
	Parse: func(p string) (*sqlx.DB, error) {
		if p == "" {
			// TODO:
			// var err error
			// if p, err = defaultDBPath(); err != nil {
			// 	return nil, err
			// }
			p = filepath.Join(os.TempDir(), "want.db")
		}
		db, err := wantdb.Open(p)
		if err != nil {
			return nil, err
		}
		if err := wantdb.Setup(context.Background(), db); err != nil {
			return nil, err
		}
		return db, nil
	},
}

// defaultDBPath sets up and returns the default path for the global DB.
func defaultDBPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	wantStateDir := filepath.Join(homeDir, ".local", "want")
	if err := os.MkdirAll(wantStateDir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(wantStateDir, "want.db"), nil
}

var projNameParam = star.Param[string]{
	Name:  "name",
	Parse: star.ParseString,
}

func openRepo() (*wantrepo.Repo, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	yes, repoPath, err := wantrepo.FindRepo(wd)
	if err != nil {
		return nil, err
	}
	if !yes {
		return nil, fmt.Errorf("%s is not in a want project", wd)
	}
	return wantrepo.Open(repoPath)
}
