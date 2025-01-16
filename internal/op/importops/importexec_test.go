package importops

import (
	"strconv"
	"testing"

	"github.com/blobcache/glfs"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"wantbuild.io/want/internal/stores"
	"wantbuild.io/want/internal/testutil"
	"wantbuild.io/want/internal/wantjob"
)

func TestImportURL(t *testing.T) {
	t.Parallel()
	//t.Skip()
	tcs := []ImportURLTask{
		{
			URL:        "https://dl-cdn.alpinelinux.org/alpine/v3.18/releases/x86_64/alpine-minirootfs-3.18.2-x86_64.tar.gz",
			Algo:       "SHA256",
			Hash:       "6c0be6213d2718087e1f4e7847e711cea288dd6cbd92c436af2c22756ac7db53",
			Transforms: []string{"ungzip", "untar"},
		},
		{
			URL:        "https://github.com/protocolbuffers/protobuf/releases/download/v24.0/protoc-24.0-linux-aarch_64.zip",
			Algo:       "SHA256",
			Hash:       "d27f1479fc4707f275eaa952cef548f9fa0c8ef2d8cb5977f337d2ce61430682",
			Transforms: []string{"unzip"},
		},
	}
	ctx := testutil.Context(t)
	for i, tc := range tcs {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			s := stores.NewMem()
			e := NewExecutor(s)
			jc := wantjob.NewCtx(ctx, nil, nil)
			y, err := e.ImportURL(ctx, &jc, s, tc)
			require.NoError(t, err)
			require.NotNil(t, y)
		})
	}
}

func TestImportGoZip(t *testing.T) {
	t.Parallel()
	//t.Skip()
	tcs := []ImportGoZipTask{
		{
			Path:    "golang.org/x/mod",
			Version: "v0.9.0",
			Hash:    "h1:KENHtAZL2y3NLMYZeHY9DW8HW8V+kQyJsY/V9JlKvCs=",
		},
	}
	ctx := testutil.Context(t)
	for i, tc := range tcs {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			s := stores.NewMem()
			e := NewExecutor(s)
			jc := wantjob.NewCtx(ctx, nil, nil)
			y, err := e.ImportGoZip(ctx, &jc, s, tc)
			require.NoError(t, err)
			require.NotNil(t, y)
		})
	}
}

func TestImportOCIImage(t *testing.T) {
	t.Parallel()
	tcs := []ImportOCIImageTask{
		{
			Name: "docker.io/library/golang",
			Algo: "sha256",
			Hash: "d9e079e899aaf93b03ee80740ffb5e98e9c182ecce42abfdfbabc029ae4d057a",
		},
		{
			Name: "docker.io/library/alpine",
			Algo: "sha256",
			Hash: "48d9183eb12a05c99bcc0bf44a003607b8e941e1d4f41f9ad12bdcc4b5672f86",
		},
	}
	ctx := testutil.Context(t)
	for i, tc := range tcs {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			s := stores.NewMem()
			e := NewExecutor(s)
			jc := wantjob.NewCtx(ctx, nil, nil)
			ref, err := e.ImportOCIImage(ctx, &jc, s, tc)
			require.NoError(t, err)
			require.NotNil(t, ref)
			// testutil.PrintFS(t, s, *ref)
		})
	}
}

func TestImportOCILayers(t *testing.T) {
	t.Parallel()
	// TODO: figure out why this gives different results than directly importing the image.
	t.Skip()
	tcs := []ImportOCIManifestTask{
		{
			Name: "docker.io/library/golang",
			Algo: "sha256",
			Hash: "d9e079e899aaf93b03ee80740ffb5e98e9c182ecce42abfdfbabc029ae4d057a",
		},
	}
	ctx := testutil.Context(t)
	for i, tc := range tcs {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			s := stores.NewMem()
			e := NewExecutor(s)
			jc := wantjob.NewCtx(ctx, nil, nil)
			mf, err := e.ImportOCIManifest(ctx, &jc, s, tc)
			require.NoError(t, err)
			require.NotNil(t, mf)

			eg, ctx := errgroup.WithContext(ctx)
			layers := make([]glfs.Ref, len(mf.Layers))
			for i, desc := range mf.Layers {
				i := i
				desc := desc
				eg.Go(func() error {
					ref, err := e.ImportOCILayer(ctx, &jc, s, ImportOCILayerTask{
						Name:       tc.Name,
						Descriptor: desc,
					})
					require.NoError(t, err)
					require.NotNil(t, ref)
					layers[i] = *ref
					return nil
				})
			}
			require.NoError(t, eg.Wait())
			ref, err := e.MergeOCILayers(ctx, &jc, s, MergeOCILayersTask{
				Layers: layers,
			})
			require.NoError(t, err)
			require.NotNil(t, ref)
			testutil.PrintFS(t, s, *ref)
		})
	}
}
