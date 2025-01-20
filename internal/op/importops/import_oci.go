package importops

import (
	"archive/tar"
	"context"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/blobcache/glfs"
	"github.com/blobcache/glfs/glfstar"
	"github.com/google/go-containerregistry/pkg/authn"
	crname "github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/lib/wantjob"
)

type ImportOCIImageTask struct {
	Name string `json:"name"`
	Algo string `json:"algo"`
	Hash string `json:"hash"`
}

func GetImportOCIImageTask(ctx context.Context, s cadata.Getter, x glfs.Ref) (*ImportOCIImageTask, error) {
	return loadJSON[ImportOCIImageTask](ctx, s, x)
}

func (e *Executor) ImportOCIImage(jc wantjob.Ctx, s cadata.Getter, task ImportOCIImageTask) (*glfs.Ref, error) {
	ag := glfs.NewAgent()
	if len(task.Hash) < 40 {
		return nil, fmt.Errorf("hash too short %q len=%d", task.Hash, len(task.Hash))
	}
	if _, err := hex.DecodeString(task.Hash); err != nil {
		return nil, err
	}
	ref, err := crname.ParseReference(fmt.Sprintf("%s@%s:%s", task.Name, task.Algo, task.Hash), crname.StrictValidation)
	if err != nil {
		return nil, err
	}
	img, err := remote.Image(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return nil, fmt.Errorf("reading image failed: %w", err)
	}
	jc.Infof("oci: %v", ref)
	rc := mutate.Extract(img)
	defer rc.Close()
	tr := tar.NewReader(rc)
	ctx := jc.Context
	return glfstar.ReadTAR(ctx, ag, jc.Dst, tr)
}

type ImportOCIManifestTask struct {
	Name string
	Algo string
	Hash string
}

func (e *Executor) ImportOCIManifest(ctx context.Context, _ *wantjob.Ctx, s cadata.Getter, task ImportOCIManifestTask) (*v1.Manifest, error) {
	if len(task.Hash) < 40 {
		return nil, fmt.Errorf("hash too short %q len=%d", task.Hash, len(task.Hash))
	}
	if _, err := hex.DecodeString(task.Hash); err != nil {
		return nil, err
	}
	ref, err := crname.ParseReference(fmt.Sprintf("%s@%s:%s", task.Name, task.Algo, task.Hash), crname.StrictValidation)
	if err != nil {
		return nil, err
	}
	// Create an anonymous (unauthenticated) client
	img, err := remote.Image(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return nil, fmt.Errorf("reading image failed: %w", err)
	}
	// Fetch and print the manifest
	manifest, err := img.Manifest()
	if err != nil {
		return nil, fmt.Errorf("fetching manifest failed: %w", err)
	}
	return manifest, nil
}

type ImportOCILayerTask struct {
	Name       string
	Descriptor v1.Descriptor
}

func (e *Executor) ImportOCILayer(jc *wantjob.Ctx, dst cadata.Store, s cadata.Getter, task ImportOCILayerTask) (*glfs.Ref, error) {
	jc.Infof("name: %v", task.Name)
	jc.Infof("descriptor: %v", task.Descriptor)
	dig, err := crname.NewDigest(task.Name+"@"+task.Descriptor.Digest.String(), crname.StrictValidation)
	if err != nil {
		return nil, err
	}
	layer, err := remote.Layer(dig, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return nil, err
	}
	rc, err := layer.Uncompressed()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	ag := glfs.NewAgent()
	tr := tar.NewReader(rc)
	ctx := jc.Context
	return glfstar.ReadTAR(ctx, ag, dst, tr)
}

type MergeOCILayersTask struct {
	Layers []glfs.Ref
}

func (e *Executor) MergeOCILayers(jc *wantjob.Ctx, dst cadata.GetPoster, s cadata.Getter, task MergeOCILayersTask) (*glfs.Ref, error) {
	ctx := jc.Context
	ag := glfs.NewAgent()
	img := empty.Image
	for _, layer := range task.Layers {
		l, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
			rc := invertStream(func(w io.Writer) error {
				tw := tar.NewWriter(w)
				if err := glfstar.WriteTAR(ctx, ag, s, layer, tw); err != nil {
					return err
				}
				return tw.Close()
			})
			return rc, nil
		})
		if err != nil {
			return nil, err
		}
		img, err = mutate.AppendLayers(img, l)
		if err != nil {
			return nil, err
		}
	}
	rc := mutate.Extract(img)
	defer rc.Close()
	tr := tar.NewReader(rc)
	return glfstar.ReadTAR(ctx, ag, dst, tr)
}

func invertStream(fn func(w io.Writer) error) io.ReadCloser {
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		if err := fn(pw); err != nil {
			pw.CloseWithError(err)
			return
		}
	}()
	return pr
}
