package importops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/blobcache/glfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/glfsgit"
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

func (e *Executor) ImportGit(ctx context.Context, dst cadata.PostExister, spec ImportGitTask) (*glfs.Ref, error) {
	if len(spec.CommitHash) < 40 {
		return nil, fmt.Errorf("invalid commit_hash %q, len=%d", spec.CommitHash, len(spec.CommitHash))
	}
	gstor := memory.NewStorage()
	r := git.NewRemote(gstor, &config.RemoteConfig{
		Name: "origin",
		URLs: []string{spec.URL},
	})
	if err := r.Fetch(&git.FetchOptions{
		RemoteName: "origin",
		Depth:      1,
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("%s:%s", spec.CommitHash, spec.CommitHash)),
		},
	}); err != nil {
		return nil, err
	}
	h := plumbing.NewHash(spec.CommitHash)
	co, exists := gstor.Commits[h]
	if !exists {
		return nil, fmt.Errorf("could not find commit %v", h)
	}
	commit, err := object.DecodeCommit(gstor, co)
	if err != nil {
		return nil, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	return glfsgit.ImportTree(ctx, dst, gstor, tree)
}
