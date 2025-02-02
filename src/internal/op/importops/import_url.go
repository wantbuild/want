package importops

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/blobcache/glfs"
	"github.com/blobcache/glfs/glfstar"
	"github.com/blobcache/glfs/glfszip"
	"github.com/klauspost/compress/zstd"
	"github.com/pkg/errors"
	"github.com/ulikunitz/xz"
	"go.brendoncarroll.net/state/cadata"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/sha3"
	"golang.org/x/sync/errgroup"
	"lukechampine.com/blake3"

	"wantbuild.io/want/src/internal/wantdag"
	"wantbuild.io/want/src/wantjob"
)

// ImportURLTask is a spec for importing data from a URL
type ImportURLTask struct {
	URL        string   `json:"url"`
	Algo       string   `json:"algo"`
	Hash       string   `json:"hash"`
	Transforms []string `json:"transforms"`
}

// PostImportURLSpec marshals spec and posts it to the store, returning a Blob Ref.
func PostImportURLTask(ctx context.Context, s cadata.Poster, spec ImportURLTask) (*glfs.Ref, error) {
	data, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	return glfs.PostBlob(ctx, s, bytes.NewReader(data))
}

// GetImportURLTask retrieves the Blob at x, and unmarshals it into an ImportURLSpec
func GetImportURLTask(ctx context.Context, s cadata.Getter, x glfs.Ref) (*ImportURLTask, error) {
	return loadJSON[ImportURLTask](ctx, s, x)
}

type GraphBuilder interface {
	Fact(ctx context.Context, v glfs.Ref) (wantdag.NodeID, error)
	Derived(ctx context.Context, op wantjob.OpName, inputs []wantdag.NodeInput) (wantdag.NodeID, error)
}

func DeriveFromURL(ctx context.Context, gb GraphBuilder, s cadata.Poster, x ImportURLTask) (wantdag.NodeID, error) {
	ref, err := PostImportURLTask(ctx, s, x)
	if err != nil {
		return 0, err
	}
	nid, err := gb.Fact(ctx, *ref)
	if err != nil {
		return 0, err
	}
	return gb.Derived(ctx, OpFromURL, []wantdag.NodeInput{
		{Name: "", Node: nid},
	})
}

// ImportURL imports data from a URL, checking that it matches a hash, and applying any transformations.
func (e *Executor) ImportURL(jc wantjob.Ctx, spec ImportURLTask) (*glfs.Ref, error) {
	u, err := url.Parse(spec.URL)
	if err != nil {
		return nil, err
	}
	newHash, err := makeHashFactory(spec.Algo)
	if err != nil {
		return nil, err
	}
	sum, err := parseHash(newHash().Size(), spec.Hash)
	if err != nil {
		return nil, err
	}
	var stages = []pipelineStage{
		checkHash(newHash(), sum),
	}
	r, err := e.download(jc, u)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return importStream(jc.Context, jc.Dst, r, stages, spec.Transforms)
}

type pipelineStage = func(w io.Writer, r io.Reader) error

func importStream(ctx context.Context, dst cadata.PostExister, r io.Reader, stages []pipelineStage, transforms []string) (*glfs.Ref, error) {
	ag := glfs.NewAgent()
	var collect = func(r io.Reader) (*glfs.Ref, error) {
		return glfs.PostBlob(ctx, dst, r)
	}
	for i, tfName := range transforms {
		switch tfName {
		case TransformUngzip:
			stages = append(stages, ungzip)
		case TransformUnxz:
			stages = append(stages, unxz)
		case TransformUnzstd:
			stages = append(stages, unzstd)
		case "unbzip2":
			stages = append(stages, unbzip2)

		case TransformUntar:
			if i != len(transforms)-1 {
				return nil, fmt.Errorf("untar must be last transform")
			}
			collect = func(r io.Reader) (*glfs.Ref, error) {
				return untar(ctx, ag, dst, r)
			}
		case TransformUnzip:
			if i != len(transforms)-1 {
				return nil, fmt.Errorf("unzip must be last transform")
			}
			collect = func(r io.Reader) (*glfs.Ref, error) {
				return unzip(ctx, ag, dst, r)
			}

		default:
			return nil, fmt.Errorf("invalid transform: %q", tfName)
		}
	}
	return pipeline(r, stages, collect)
}

func ungzip(w io.Writer, r io.Reader) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()
	_, err = io.Copy(w, gz)
	return err
}

func unxz(w io.Writer, r io.Reader) error {
	xzr, err := xz.NewReader(r)
	if err != nil {
		return err
	}
	xzr.SingleStream = true
	if _, err := io.Copy(w, xzr); err != nil {
		return err
	}
	return nil
}

func unzstd(w io.Writer, r io.Reader) error {
	zr, err := zstd.NewReader(r)
	if err != nil {
		return err
	}
	defer zr.Close()
	_, err = io.Copy(w, zr)
	return err
}

func unbzip2(w io.Writer, r io.Reader) error {
	zr := bzip2.NewReader(r)
	_, err := io.Copy(w, zr)
	return err
}

func checkHash(h hash.Hash, sum []byte) pipelineStage {
	return func(w io.Writer, r io.Reader) error {
		h.Reset()
		mw := io.MultiWriter(w, h)
		if _, err := io.Copy(mw, r); err != nil {
			return err
		}
		actual := h.Sum(nil)
		if !bytes.Equal(actual, sum) {
			return errors.Errorf("import-url: checksum does not match. HAVE: %x WANT: %x", actual, sum)
		}
		return nil
	}
}

func untar(ctx context.Context, op *glfs.Agent, s cadata.PostExister, r io.Reader) (*glfs.Ref, error) {
	tr := tar.NewReader(r)
	ref, err := glfstar.ReadTAR(ctx, op, s, tr)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(io.Discard, r); err != nil {
		return nil, err
	}
	return ref, nil
}

func unzip(ctx context.Context, op *glfs.Agent, s cadata.PostExister, r io.Reader) (*glfs.Ref, error) {
	f, err := os.CreateTemp("", "unzip-*")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := os.Remove(f.Name()); err != nil {
		return nil, err
	}
	size, err := io.Copy(f, r)
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(f, size)
	if err != nil {
		return nil, err
	}
	return glfszip.Import(ctx, op, s, zr)
}

func pipeline(r io.Reader, stages []func(w io.Writer, r io.Reader) error, collect func(r io.Reader) (*glfs.Ref, error)) (*glfs.Ref, error) {
	eg := errgroup.Group{}
	// stages
	for i := range stages {
		stage := stages[i]
		r2 := r
		pr, pw := io.Pipe()
		eg.Go(func() error {
			if err := stage(pw, r2); err != nil {
				pw.CloseWithError(err)
				return err
			}
			return pw.Close()
		})
		r = pr
	}
	// end
	var ret *glfs.Ref
	eg.Go(func() error {
		ref, err := collect(r)
		if err != nil {
			return err
		}
		ret = ref
		return nil
	})
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return ret, nil
}

func (e *Executor) download(jc wantjob.Ctx, u *url.URL) (io.ReadCloser, error) {
	switch u.Scheme {
	case "http", "https":
		return e.downloadHTTP(jc, u)
	default:
		return nil, errors.New("url scheme not supported")
	}
}

func (e *Executor) downloadHTTP(jc wantjob.Ctx, u *url.URL) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(jc.Context, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	res, err := e.hc.Do(req)
	if err != nil {
		return nil, err
	}
	jc.Infof("http %v %v", res.Status, u.String())
	if res.StatusCode != 200 {
		res.Body.Close()
		return nil, errors.Errorf("got non 200 status %s", res.Status)
	}
	return res.Body, nil
}

func makeHashFactory(algo string) (func() hash.Hash, error) {
	var y func() hash.Hash
	switch algo {
	case "SHA1":
		y = sha1.New
	case "SHA256", "SHA2-256":
		y = sha256.New
	case "SHA512", "SHA2-512":
		y = sha512.New
	case "SHA3-256":
		y = sha3.New256
	case "SHA3-512":
		y = sha3.New512
	case "BLAKE2b-256":
		y = func() hash.Hash {
			h, err := blake2b.New(32, nil)
			if err != nil {
				panic(err)
			}
			return h
		}
	case "BLAKE3-256":
		y = func() hash.Hash { return blake3.New(32, nil) }
	default:
		return nil, errors.Errorf("unsupported hash function %q", algo)
	}
	return y, nil
}

func parseHash(size int, hash string) ([]byte, error) {
	switch len(hash) {
	case 0:
		return nil, nil
	case hex.EncodedLen(size):
		return hex.DecodeString(hash)
	case base64.StdEncoding.EncodedLen(size):
		return base64.StdEncoding.DecodeString(hash)
	default:
		return nil, errors.Errorf("could not figure out encoding for hash (%s) len=%d", hash, len(hash))
	}
}
