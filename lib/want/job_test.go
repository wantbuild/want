package want

import (
	"runtime"
	"strings"
	"testing"

	"github.com/blobcache/glfs"
	"github.com/stretchr/testify/require"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/glfstasks"
	"wantbuild.io/want/internal/stores"
	"wantbuild.io/want/internal/testutil"
	"wantbuild.io/want/internal/wantdb"
	"wantbuild.io/want/lib/wantjob"
)

func TestJobStores(t *testing.T) {
	ctx := testutil.Context(t)
	db := wantdb.NewMemory()
	require.NoError(t, wantdb.Setup(ctx, db))
	exec := testExecutor{
		"op1": func(jc *wantjob.Ctx, dst cadata.Store, src cadata.Getter, x []byte) ([]byte, error) {
			ctx := jc.Context()
			// generate some data in a tree which will need to be synced.
			m := map[string]glfs.Ref{}
			for _, k := range []string{"a", "b", "c"} {
				ref, err := glfs.PostBlob(ctx, dst, strings.NewReader(strings.Repeat(k, 100)))
				require.NoError(t, err)
				m[k] = *ref
			}
			ref, err := glfs.PostTreeMap(ctx, dst, m)
			if err != nil {
				return nil, err
			}
			return glfstasks.MarshalGLFSRef(*ref), nil
		},
	}
	jsys := newJobSys(ctx, db, exec, runtime.GOMAXPROCS(0))
	defer jsys.Shutdown()

	s := stores.NewMem()
	idx, err := jsys.Init(ctx, s, wantjob.Task{Op: "op1"})
	require.NoError(t, err)
	require.NoError(t, jsys.Await(ctx, nil, idx))

	res, s2, err := jsys.ViewResult(ctx, nil, idx)
	require.NoError(t, err)
	require.NoError(t, res.Err())
	ref, err := glfstasks.ParseGLFSRef(res.Data)
	require.NoError(t, err)
	var count int
	require.NoError(t, glfs.WalkRefs(ctx, s2, *ref, func(ref glfs.Ref) error { count++; return nil }))
	require.Equal(t, 4, count)
}

type testExecutor map[wantjob.OpName]wantjob.OpFunc

func (te testExecutor) Execute(jc *wantjob.Ctx, dst cadata.Store, src cadata.Getter, task wantjob.Task) ([]byte, error) {
	fn := te[task.Op]
	if fn == nil {
		return nil, wantjob.NewErrUnknownOperator(task.Op)
	}
	return fn(jc, dst, src, task.Input)
}
