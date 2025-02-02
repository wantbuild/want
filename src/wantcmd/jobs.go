package wantcmd

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.brendoncarroll.net/star"
	"wantbuild.io/want/src/wantjob"
)

var jobCmd = star.NewDir(star.Metadata{
	Short: "inspect and manage jobs",
}, map[star.Symbol]star.Command{
	"ls":   lsJobCmd,
	"drop": dropJobCmd,
})

var lsJobCmd = star.Command{
	Metadata: star.Metadata{Short: "list jobs"},
	F: func(c star.Context) error {
		ctx := c.Context
		wbs, err := newSys(&c)
		if err != nil {
			return err
		}
		defer wbs.Close()

		jobs, err := wbs.ListJobInfos(ctx)
		if err != nil {
			return err
		}
		w := c.StdOut
		fmtStr := "%-8s %-16s %-24s %-24s\n"
		fmt.Fprintf(w, fmtStr, "ID", "OP", "CREATED_AT", "END_AT")
		for _, j := range jobs {
			createdAt := j.CreatedAt.GoTime().Format(time.RFC3339)
			var endAt string
			if j.EndAt != nil {
				endAt = j.EndAt.GoTime().Format(time.RFC3339)
			}
			fmt.Fprintf(w, fmtStr, j.ID, j.Task.Op, createdAt, endAt)
		}
		return w.Flush()
	},
}

var dropJobCmd = star.Command{
	Metadata: star.Metadata{Short: "drop a job"},
	Pos:      []star.IParam{jobidParam},
	F: func(c star.Context) error {
		wbs, err := newSys(&c)
		if err != nil {
			return err
		}
		defer wbs.Close()
		jobid := jobidParam.Load(c)
		return wbs.JobSystem().Delete(c.Context, jobid[0])
	},
}

var dashCmd = star.Command{
	Metadata: star.Metadata{Short: "serve dashboard on localhost"},
	F: func(c star.Context) error {
		wbs, err := newSys(&c)
		if err != nil {
			return err
		}
		defer wbs.Close()

		fsys := wbs.LogFS()
		l, err := net.Listen("tcp4", "127.0.0.1:8420")
		if err != nil {
			return err
		}
		c.Printf("listening on http://%v ...\n", l.Addr())
		c.StdOut.Flush()
		return http.Serve(l, http.FileServerFS(fsys))
	},
}

var jobidParam = star.Param[wantjob.JobID]{
	Parse: func(x string) (wantjob.JobID, error) {
		parts := strings.Split(strings.Trim(x, "/"), "/")
		var ret wantjob.JobID
		for _, part := range parts {
			n, err := strconv.ParseUint(part, 16, 32)
			if err != nil {
				return nil, err
			}
			ret = append(ret, wantjob.Idx(n))
		}
		if len(ret) == 0 {
			return nil, fmt.Errorf("empty job id")
		}
		return ret, nil
	},
}
