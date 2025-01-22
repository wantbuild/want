package wantcmd

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"go.brendoncarroll.net/star"
)

var jobCmd = star.NewDir(star.Metadata{
	Short: "inspect and manage jobs",
}, map[star.Symbol]star.Command{
	"ls": lsJobCmd,
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
