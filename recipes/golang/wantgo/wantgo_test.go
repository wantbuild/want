package wantgo

import (
	"bytes"
	"context"
	"io"
	"os"
	"path"
	"testing"

	"github.com/blobcache/glfs"
	"github.com/kr/text"
	"github.com/stretchr/testify/require"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/want/testwant"
	"wantbuild.io/want/src/wantjob"
)

func TestPostGetRunTestsTask(t *testing.T) {
	ctx := context.TODO()
	s := newStore()

	x := RunTestsTask{
		Module: mustPostFS(t, s, map[string][]byte{
			"go.mod": []byte("module thing1"),
			"go.sum": []byte{},
			"main.go": []byte(`package main
				func main() {}
			`),
		}),
		VMSpec: VMSpec{
			Kernel: mustPostBlob(t, s, nil),
			BaseFS: mustPostFS(t, s, map[string][]byte{}),
		},
	}
	ref, err := PostRunTestsTask(ctx, s, x)
	require.NoError(t, err)
	y, err := GetRunTestsTask(ctx, s, *ref)
	require.NoError(t, err)
	require.Equal(t, x, *y)
}

func TestRunTests(t *testing.T) {
	jc := newJobCtx(t)
	s := jc.Dst
	bzImage := loadKernel(t)
	ref, err := RunTests(jc, s, RunTestsTask{
		Module: mustPostFS(t, s, map[string][]byte{
			"go.mod": []byte("module example_module \n"),
			"go.sum": []byte(""),
			"subpkg1/a_test.go": []byte(`package subpkg1
				import (
					"testing"
					"fmt"
				)
				
				func TestA(t *testing.T) {
					fmt.Println("Hello from Test A")
				}
			`),
			"subpkg1/subpkg2/b_test.go": []byte(`package subpkg2
				import (
					"testing"
					"fmt"
				)
				
				func TestBGood(t *testing.T) {
					fmt.Println("Hello from TestBGood")
				}
				
				func TestBBad(t *testing.T) {
					t.Error("hello from TestBBad")			
				}
			`),
		}),
		VMSpec: VMSpec{
			BaseFS: mustPostFS(t, s, nil),
			Kernel: mustPostBlob(t, s, bzImage),
		},
		RunTestsConfig: RunTestsConfig{
			GOARCH: "amd64",
			GOOS:   "linux",
		},
	})
	require.NoError(t, err)
	ctx := jc.Context
	require.NoError(t, glfs.WalkTree(ctx, s, *ref, func(prefix string, ent glfs.TreeEntry) error {
		p := path.Join(prefix, ent.Name)
		t.Log(p, ent.FileMode, ent.Ref)
		if ent.Ref.Type == glfs.TypeBlob {
			data, err := glfs.GetBlobBytes(ctx, s, ent.Ref, 1e6)
			if err != nil {
				return err
			}
			t.Log(string(data))
		}
		return nil
	}))
	requirePathExists(t, s, *ref, "subpkg1.test")
	requirePathExists(t, s, *ref, "subpkg1/subpkg2.test")
}

func requirePathExists(t testing.TB, s cadata.Getter, x glfs.Ref, k string) glfs.Ref {
	ref, err := glfs.GetAtPath(context.TODO(), s, x, k)
	require.NoError(t, err)
	return *ref
}

func mustPostFS(t testing.TB, s cadata.PostExister, m map[string][]byte) glfs.Ref {
	ctx := context.TODO()
	ref, err := postFS(ctx, s, m)
	require.NoError(t, err)
	return *ref
}

func mustPostBlob(t testing.TB, s cadata.Poster, data []byte) glfs.Ref {
	ctx := context.TODO()
	ref, err := glfs.PostBlob(ctx, s, bytes.NewReader(data))
	require.NoError(t, err)
	return *ref
}

func newStore() cadata.Store {
	return cadata.NewMem(cadata.DefaultHash, 1<<21)
}

func newJobCtx(t testing.TB) wantjob.Ctx {
	ctx := context.TODO()
	sys := testwant.NewSystem(t)
	return wantjob.Ctx{
		Context: ctx,
		System:  sys.JobSystem(),
		Dst:     newStore(),
		Writer: func(s string) io.Writer {
			return text.NewIndentWriter(os.Stderr, []byte(s+"|"))
		},
	}
}

func postFS(ctx context.Context, s cadata.PostExister, m map[string][]byte) (*glfs.Ref, error) {
	var ents []glfs.TreeEntry
	for k, v := range m {
		ref, err := glfs.PostBlob(ctx, s, bytes.NewReader(v))
		if err != nil {
			return nil, err
		}
		ents = append(ents, glfs.TreeEntry{
			Name:     k,
			FileMode: 0o777,
			Ref:      *ref,
		})
	}
	return glfs.PostTreeSlice(ctx, s, ents)
}

func loadKernel(t testing.TB) []byte {
	data, err := os.ReadFile("./bzImage")
	require.NoError(t, err)
	return data
}
