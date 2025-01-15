package importops

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/stores"
	"wantbuild.io/want/internal/wantjob"
)

const (
	OpFromURL      = wantjob.OpName("fromURL")
	OpFromGit      = wantjob.OpName("fromGit")
	OpFromGoZip    = wantjob.OpName("fromGoZip")
	OpFromOCIImage = wantjob.OpName("fromOCIImage")

	OpUnpack = wantjob.OpName("unpack")
)

var _ wantjob.Executor = &Executor{}

type Executor struct {
	s  cadata.GetPoster
	hc *http.Client
}

func NewExecutor(s cadata.GetPoster) *Executor {
	return &Executor{s: s, hc: http.DefaultClient}
}

func (e *Executor) Compute(ctx context.Context, jc *wantjob.Ctx, s cadata.Getter, x wantjob.Task) (*glfs.Ref, error) {
	s2 := stores.Fork{W: e.s, R: s}
	switch x.Op {
	case OpFromURL:
		spec, err := GetImportURLTask(ctx, s, x.Input)
		if err != nil {
			return nil, err
		}
		return e.ImportURL(ctx, jc, s2, *spec)
	case OpFromGit:
		spec, err := GetImportGitTask(ctx, s, x.Input)
		if err != nil {
			return nil, err
		}
		return e.ImportGit(ctx, s2, *spec)
	case OpUnpack:
		spec, err := GetUnpackTask(ctx, s, x.Input)
		if err != nil {
			return nil, err
		}
		return e.Unpack(ctx, s2, *spec)
	case OpFromGoZip:
		spec, err := GetImportGoZipTask(ctx, s, x.Input)
		if err != nil {
			return nil, err
		}
		return e.ImportGoZip(ctx, jc, s2, *spec)
	case OpFromOCIImage:
		spec, err := GetImportOCIImageTask(ctx, s, x.Input)
		if err != nil {
			return nil, err
		}
		return e.ImportOCIImage(ctx, jc, s, *spec)
	default:
		return nil, wantjob.NewErrUnknownOperator(x.Op)
	}
}

func (e *Executor) GetStore() cadata.Getter {
	return e.s
}

const MaxConfigSize = 1e6

func loadJSON[T any](ctx context.Context, s cadata.Getter, ref glfs.Ref) (*T, error) {
	data, err := glfs.GetBlobBytes(ctx, s, ref, MaxConfigSize)
	if err != nil {
		return nil, err
	}
	var ret T
	if err := json.Unmarshal(data, &ret); err != nil {
		return nil, err
	}
	return &ret, nil
}
