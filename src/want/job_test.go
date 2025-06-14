package want

import (
	"runtime"
	"strings"
	"testing"

	"blobcache.io/glfs"
	"github.com/stretchr/testify/require"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/internal/testutil"
	"wantbuild.io/want/src/internal/wantdb"
	"wantbuild.io/want/src/wantjob"
	"wantbuild.io/want/src/wantjob/wantjobtests"
)

func TestJobSuite(t *testing.T) {
	wantjobtests.TestJobs(t, func(t testing.TB, exec wantjob.Executor) wantjob.System {
		ctx := testutil.Context(t)
		db := wantdb.NewMemory()
		require.NoError(t, wantdb.Setup(ctx, db))
		return newJobSystem(db, t.TempDir(), exec, 1)
	})
}

func TestJobStores(t *testing.T) {
	ctx := testutil.Context(t)
	db := wantdb.NewMemory()
	require.NoError(t, wantdb.Setup(ctx, db))
	exec := wantjob.BasicExecutor{
		"op1": func(jc wantjob.Ctx, src cadata.Getter, x []byte) wantjob.Result {
			return glfstasks.Exec(x, func(x glfs.Ref) (*glfs.Ref, error) {
				ctx := jc.Context
				// generate some data in a tree which will need to be synced.
				m := map[string]glfs.Ref{}
				for _, k := range []string{"a", "b", "c"} {
					ref, err := glfs.PostBlob(ctx, jc.Dst, strings.NewReader(strings.Repeat(k, 100)))
					require.NoError(t, err)
					m[k] = *ref
				}
				return glfs.PostTreeMap(ctx, jc.Dst, m)
			})
		},
	}
	logDir := t.TempDir()
	jsys := newJobSystem(db, logDir, exec, runtime.GOMAXPROCS(0))
	defer jsys.Shutdown()

	s := stores.NewMem()
	idx, err := jsys.Spawn(ctx, s, wantjob.Task{Op: "op1", Input: []byte("{}")})
	require.NoError(t, err)
	require.NoError(t, jsys.Await(ctx, idx))

	res, s2, err := jsys.ViewResult(ctx, idx)
	require.NoError(t, err)
	require.NoError(t, res.Err())
	ref, err := glfstasks.ParseGLFSRef(res.Root)
	require.NoError(t, err)
	var count int
	require.NoError(t, glfs.WalkRefs(ctx, s2, *ref, func(ref glfs.Ref) error { count++; return nil }))
	require.Equal(t, 4, count)
}
