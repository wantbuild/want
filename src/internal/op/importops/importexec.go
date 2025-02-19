package importops

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/wantjob"
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
	hc *http.Client
}

func NewExecutor() *Executor {
	return &Executor{hc: http.DefaultClient}
}

func (e *Executor) Execute(jc wantjob.Ctx, s cadata.Getter, x wantjob.Task) wantjob.Result {
	ctx := jc.Context

	switch x.Op {
	case OpFromURL:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			spec, err := GetImportURLTask(ctx, s, x)
			if err != nil {
				return nil, err
			}
			return e.ImportURL(jc, *spec)
		})
	case OpFromGit:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			spec, err := GetImportGitTask(ctx, s, x)
			if err != nil {
				return nil, err
			}
			return e.ImportGit(ctx, jc.Dst, *spec)
		})
	case OpUnpack:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			spec, err := GetUnpackTask(ctx, s, x)
			if err != nil {
				return nil, err
			}
			return e.Unpack(ctx, jc.Dst, s, *spec)
		})
	case OpFromGoZip:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			spec, err := GetImportGoZipTask(ctx, s, x)
			if err != nil {
				return nil, err
			}
			return e.ImportGoZip(jc, jc.Dst, *spec)
		})
	case OpFromOCIImage:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			spec, err := GetImportOCIImageTask(ctx, s, x)
			if err != nil {
				return nil, err
			}
			return e.ImportOCIImage(jc, s, *spec)
		})
	default:
		return *wantjob.Result_ErrExec(wantjob.NewErrUnknownOperator(x.Op))
	}
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
