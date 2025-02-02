package want

import (
	"context"

	"go.brendoncarroll.net/stdctx/logctx"
	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/internal/wantdb"
)

func (sys *System) Scrub(ctx context.Context) error {

	// check stores
	logctx.Infof(ctx, "checking all stores")
	var rows []struct {
		StoreID wantdb.StoreID `db:"store_id"`
		ResData []byte         `db:"res_data"`
	}
	if err := sys.db.Select(&rows, `SELECT distinct store_id, res_data FROM jobs WHERE state = 3 AND errcode = 0`); err != nil {
		return err
	}
	for i, row := range rows {
		logctx.Infof(ctx, "checking store %d/%d: store_id=%d", i+1, len(rows), row.StoreID)
		if ref, err := glfstasks.ParseGLFSRef(row.ResData); err == nil {
			s := wantdb.NewDBStore(sys.db, row.StoreID)
			if err := glfstasks.Check(ctx, s, *ref); err != nil {
				logctx.Errorf(ctx, "store_id=%v failed integrity check: %v", s.StoreID(), err)
			}
		}
	}
	return nil
}
