package wantcmd

import (
	"fmt"
	"os"
	"runtime"

	"go.brendoncarroll.net/star"

	"wantbuild.io/want/src/want"
	"wantbuild.io/want/src/wantrepo"
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

		"job":        jobCmd,
		"dash":       dashCmd,
		"serve-http": serveHttpCmd,
		"scrub":      *scrubCmd,
	},
)

var initCmd = star.Command{
	Metadata: star.Metadata{Short: "initialize want in the current directory"},
	Pos:      []star.IParam{},
	F: func(c star.Context) error {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		return wantrepo.Init(wd)
	},
}

var statusCmd = star.Command{
	Metadata: star.Metadata{Short: "print status information"},
	Flags:    []star.IParam{},
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

var scrubCmd = &star.Command{
	F: func(c star.Context) error {
		wbs, err := newSys(&c)
		if err != nil {
			return err
		}
		return wbs.Scrub(c.Context)
	},
}

func newSys(c *star.Context) (*want.System, error) {
	const stateDir = "/tmp/want"
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, err
	}
	s := want.New(stateDir, runtime.GOMAXPROCS(0))
	if err := s.Init(c.Context); err != nil {
		return nil, err
	}
	return s, nil
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
