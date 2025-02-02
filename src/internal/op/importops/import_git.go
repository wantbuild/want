package importops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/blobcache/glfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/glfsport"
)

type ImportGitTask struct {
	URL        string `json:"url"`
	Branch     string `json:"branch"`
	CommitHash string `json:"commitHash"`
}

// PostImportGitTask marshals spec and posts it to the store, returning a Blob Ref.
func PostImportGitTask(ctx context.Context, s cadata.Poster, spec ImportGitTask) (*glfs.Ref, error) {
	data, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	return glfs.PostBlob(ctx, s, bytes.NewReader(data))
}

// GetImportGitTask retrieves the Blob at x, and unmarshals it into an ImportGitTask
func GetImportGitTask(ctx context.Context, s cadata.Getter, x glfs.Ref) (*ImportGitTask, error) {
	return loadJSON[ImportGitTask](ctx, s, x)
}

func (e *Executor) ImportGit(ctx context.Context, s cadata.PostExister, spec ImportGitTask) (*glfs.Ref, error) {
	if len(spec.CommitHash) < 40 {
		return nil, fmt.Errorf("invalid commit_hash %q, len=%d", spec.CommitHash, len(spec.CommitHash))
	}
	dir, err := os.MkdirTemp("", "git-clone-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	r, err := git.PlainCloneContext(ctx, dir, false, &git.CloneOptions{
		URL:           spec.URL,
		Depth:         1,
		SingleBranch:  true,
		NoCheckout:    true,
		ReferenceName: plumbing.NewBranchReferenceName(spec.Branch),
	})
	if err != nil {
		return nil, err
	}
	wt, err := r.Worktree()
	if err != nil {
		return nil, err
	}
	if err := wt.Checkout(&git.CheckoutOptions{
		Hash: plumbing.NewHash(spec.CommitHash),
	}); err != nil {
		return nil, err
	}
	imp := glfsport.Importer{
		Cache: glfsport.NullCache{},
		Dir:   dir,
		Filter: func(p string) bool {
			isGit := strings.HasPrefix(p, ".git/") || p == ".git"
			return !isGit
		},
		Store: s,
	}
	return imp.Import(ctx, "")
}
