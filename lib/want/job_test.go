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
	exec := wantjob.BasicExecutor{
		"op1": func(jc wantjob.Ctx, src cadata.Getter, x []byte) ([]byte, error) {
			ctx := jc.Context
			// generate some data in a tree which will need to be synced.
			m := map[string]glfs.Ref{}
			for _, k := range []string{"a", "b", "c"} {
				ref, err := glfs.PostBlob(ctx, jc.Dst, strings.NewReader(strings.Repeat(k, 100)))
				require.NoError(t, err)
				m[k] = *ref
			}
			ref, err := glfs.PostTreeMap(ctx, jc.Dst, m)
			if err != nil {
				return nil, err
			}
			return glfstasks.MarshalGLFSRef(*ref), nil
		},
	}
	jsys := newJobSystem(db, exec, runtime.GOMAXPROCS(0))
	defer jsys.Shutdown()

	s := stores.NewMem()
	idx, err := jsys.Spawn(ctx, s, wantjob.Task{Op: "op1"})
	require.NoError(t, err)
	require.NoError(t, jsys.Await(ctx, idx))

	res, s2, err := jsys.ViewResult(ctx, idx)
	require.NoError(t, err)
	require.NoError(t, res.Err())
	ref, err := glfstasks.ParseGLFSRef(res.Data)
	require.NoError(t, err)
	var count int
	require.NoError(t, glfs.WalkRefs(ctx, s2, *ref, func(ref glfs.Ref) error { count++; return nil }))
	require.Equal(t, 4, count)
}
