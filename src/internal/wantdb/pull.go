package wantdb

import (
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

var ErrPullNoMatch = errors.New("could not pull, no match")

// Pull finds a job with result = root, uses its store as a source and performs a CopyAll
func Pull(tx *sqlx.Tx, dstID StoreID, root []byte) error {
	var srcID StoreID
	if err := tx.Get(&srcID, `SELECT store_id FROM jobs
		WHERE state = 3 AND errcode = 0 AND res_data = ?`, root); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPullNoMatch
		}
		return err
	}
	return CopyAll(tx, srcID, dstID)
}
