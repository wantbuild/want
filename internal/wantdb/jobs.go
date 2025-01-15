package wantdb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/dbutil"
	"wantbuild.io/want/internal/wantjob"
)

func CreateRootJob(tx *sqlx.Tx, task wantjob.Task) (wantjob.Idx, error) {
	sid, err := CreateStore(tx)
	if err != nil {
		return 0, err
	}
	taskID, err := ensureTask(tx, task)
	if err != nil {
		return 0, err
	}
	// insert information to the main jobs table
	rowid, err := dbutil.GetTx[int64](tx, `INSERT INTO jobs (task) VALUES (?) RETURNING rowid`, taskID)
	if err != nil {
		return 0, err
	}
	// create the root entry
	idx, err := dbutil.GetTx[int64](tx, `INSERT INTO job_roots (job_row, store_id) VALUES (?, ?) RETURNING idx`, rowid, sid)
	if err != nil {
		return 0, err
	}
	return wantjob.Idx(idx), nil
}

func DropRootJob(tx *sqlx.Tx, rootIdx wantjob.Idx) error {
	// TODO: iterate over children and remove them.
	var row struct {
		JobRow  int64   `db:"jobrow"`
		StoreID StoreID `db:"store_id"`
	}
	if err := tx.Get(&row, `SELECT jobrow, store_id FROM job_roots WHERE idx = ?`, rootIdx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if err := DropStore(tx, row.StoreID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM job_roots WHERE idx = ?`, rootIdx); err != nil {
		return err
	}
	return nil
}

func FinishJob(tx *sqlx.Tx, jobid wantjob.JobID, errcode wantjob.ErrCode, data []byte) error {
	// TODO: check if any children are still running
	rowid, err := lookupJobRowID(tx, jobid)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE jobs
		SET state = 3, errcode = ?, res_data = ?
		WHERE state != 3 AND rowid = ?`, errcode, data, rowid)
	return err
}

func InspectJob(tx *sqlx.Tx, jobid wantjob.JobID) (*wantjob.Job, error) {
	rowid, err := lookupJobRowID(tx, jobid)
	if err != nil {
		return nil, err
	}
	var row struct {
		State      uint32 `db:"state"`
		ErrCode    uint32 `db:"errcode"`
		ResultData []byte `db:"res_data"`
	}
	if err := tx.Get(&row, `SELECT state, res_data, errcode FROM jobs WHERE rowid = ?`, rowid); err != nil {
		return nil, err
	}
	job := wantjob.Job{}
	return &job, nil
}

func CacheRead(tx *sqlx.Tx, taskID cadata.ID) ([]byte, error) {
	var data []byte
	err := tx.Get(&data, `SELECT res_data FROM jobs
		WHERE task = ? AND state = 3 AND errcode = 0
		LIMIT 1`, taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return data, err
}

func ensureOp(tx *sqlx.Tx, name wantjob.OpName) (int64, error) {
	if id, err := dbutil.GetTx[int64](tx, `SELECT id FROM ops WHERE name = ?`, string(name)); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	} else if err == nil {
		return id, nil
	}
	return dbutil.GetTx[int64](tx, `INSERT INTO ops (name) VALUES (?) RETURNING id`, string(name))
}

func ensureTask(tx *sqlx.Tx, task wantjob.Task) (cadata.ID, error) {
	taskID := task.ID()
	opid, err := ensureOp(tx, task.Op)
	if err != nil {
		return cadata.ID{}, err
	}
	inputData, err := json.Marshal(task.Input)
	if err != nil {
		return cadata.ID{}, err
	}
	if _, err := tx.Exec(`INSERT INTO tasks (id, op, input) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`, taskID, opid, inputData); err != nil {
		return cadata.ID{}, err
	}
	return taskID, nil
}

func lookupJobRowID(tx *sqlx.Tx, jobid wantjob.JobID) (int64, error) {
	if len(jobid) == 0 {
		return 0, fmt.Errorf("empty job id")
	}
	rowid, err := dbutil.GetTx[int64](tx, `SELECT job_row FROM job_roots WHERE idx = ?`, jobid[0])
	if err != nil {
		return 0, err
	}
	jobid = jobid[1:]
	for len(jobid) > 0 {
		idx := jobid[0]
		var err error
		rowid, err = dbutil.GetTx[int64](tx, `SELECT FROM job_children WHERE parent = ? AND idx = ?`, rowid, idx)
		if err != nil {
			return 0, err
		}
		jobid = jobid[1:]
	}
	return rowid, nil
}
