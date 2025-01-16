package importops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"
	"golang.org/x/mod/module"
	"golang.org/x/mod/sumdb/dirhash"

	"wantbuild.io/want/lib/wantjob"
)

type ImportGoZipTask struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	Hash    string `json:"hash"`
}

func GetImportGoZipTask(ctx context.Context, s cadata.Getter, ref glfs.Ref) (*ImportGoZipTask, error) {
	return loadJSON[ImportGoZipTask](ctx, s, ref)
}

func MarshalGoZipTask(x ImportGoZipTask) []byte {
	data, err := json.Marshal(x)
	if err != nil {
		panic(err)
	}
	return data
}

// ImportGoZip imports and validates a go module zip file
func (e *Executor) ImportGoZip(ctx context.Context, jc *wantjob.Ctx, s cadata.GetPoster, x ImportGoZipTask) (*glfs.Ref, error) {
	const proxy = "https://proxy.golang.org"
	escapedPath, err := module.EscapePath(x.Path)
	if err != nil {
		return nil, err
	}
	escapedVersion, err := module.EscapeVersion(x.Version)
	if err != nil {
		return nil, err
	}
	rawURL := fmt.Sprintf("%s/%s/@v/%s.zip", proxy, escapedPath, escapedVersion)
	u, err := url.Parse(rawURL)
	if err != nil {
		panic(err)
	}
	rc, err := e.downloadHTTP(ctx, jc, u)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	zipFile, err := os.CreateTemp("", "go-zip-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(zipFile.Name())
	defer zipFile.Close()
	if _, err := io.Copy(zipFile, rc); err != nil {
		return nil, err
	}
	actual, err := dirhash.HashZip(zipFile.Name(), dirhash.Hash1)
	if err != nil {
		return nil, err
	}
	if actual != x.Hash {
		return nil, fmt.Errorf("dirHash for %s@%s does not match HAVE: %v, WANT: %v", x.Path, x.Version, actual, x.Hash)
	}
	if _, err := zipFile.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return importStream(ctx, s, zipFile, nil, []string{"unzip"})
}
