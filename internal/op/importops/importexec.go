package importops

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/glfstasks"
	"wantbuild.io/want/internal/stores"
	"wantbuild.io/want/lib/wantjob"
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

func (e *Executor) Execute(jc *wantjob.Ctx, dst cadata.Store, s cadata.Getter, x wantjob.Task) ([]byte, error) {
	ctx := jc.Context()
	s2 := stores.Fork{W: dst, R: s}

	switch x.Op {
	case OpFromURL:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			spec, err := GetImportURLTask(ctx, s, x)
			if err != nil {
				return nil, err
			}
			return e.ImportURL(ctx, jc, s2, *spec)
		})
	case OpFromGit:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			spec, err := GetImportGitTask(ctx, s, x)
			if err != nil {
				return nil, err
			}
			return e.ImportGit(ctx, s2, *spec)
		})
	case OpUnpack:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			spec, err := GetUnpackTask(ctx, s, x)
			if err != nil {
				return nil, err
			}
			return e.Unpack(ctx, s2, *spec)
		})
	case OpFromGoZip:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			spec, err := GetImportGoZipTask(ctx, s, x)
			if err != nil {
				return nil, err
			}
			return e.ImportGoZip(ctx, jc, s2, *spec)
		})
	case OpFromOCIImage:
		return glfstasks.Exec(x.Input, func(x glfs.Ref) (*glfs.Ref, error) {
			spec, err := GetImportOCIImageTask(ctx, s, x)
			if err != nil {
				return nil, err
			}
			return e.ImportOCIImage(jc, dst, s, *spec)
		})
	default:
		return nil, wantjob.NewErrUnknownOperator(x.Op)
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
