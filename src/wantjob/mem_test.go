package wantjob_test

import (
	"testing"

	"wantbuild.io/want/src/internal/testutil"
	"wantbuild.io/want/src/wantjob"
	"wantbuild.io/want/src/wantjob/wantjobtests"
)

func TestMemJob(t *testing.T) {
	wantjobtests.TestJobs(t, func(t testing.TB, exec wantjob.Executor) wantjob.System {
		ctx := testutil.Context(t)
		return wantjob.NewMem(ctx, exec)
	})
}
