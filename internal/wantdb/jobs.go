package wantdb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"go.brendoncarroll.net/state/cadata"
	"go.brendoncarroll.net/tai64"

	"wantbuild.io/want/internal/dbutil"
	"wantbuild.io/want/internal/wantjob"
)

func CreateRootJob(tx *sqlx.Tx, task wantjob.Task) (wantjob.Idx, error) {
	sid, err := CreateStore(tx)
	if err != nil {
		return 0, err
	}
	rowid, err := createJob(tx, sid, task)
	if err != nil {
		return 0, err
	}
	// create the root entry
	idx, err := dbutil.GetTx[int64](tx, `INSERT INTO job_roots (job_row) VALUES (?) RETURNING idx`, rowid)
	if err != nil {
		return 0, err
	}
	return wantjob.Idx(idx), nil
}

func createJob(tx *sqlx.Tx, sid StoreID, task wantjob.Task) (int64, error) {
	taskID, err := ensureTask(tx, task)
	if err != nil {
		return 0, err
	}
	now := tai64.Now()
	return dbutil.GetTx[int64](tx, `INSERT INTO jobs (task, store_id, start_at) VALUES (?, ?, ?) RETURNING rowid`, taskID, sid, now.Marshal())
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

func CreateChildJob(tx *sqlx.Tx, parentID wantjob.JobID, task wantjob.Task) (wantjob.Idx, error) {
	parentRow, err := lookupJobRowID(tx, parentID)
	if err != nil {
		return 0, err
	}
	var row struct {
		Store   StoreID     `db:"store_id"`
		NextIdx wantjob.Idx `db:"next_idx"`
	}
	if err := tx.Get(&row, `UPDATE jobs SET next_idx = next_idx + 1 WHERE rowid = ? RETURNING next_idx, store_id`, parentRow); err != nil {
		return 0, err
	}
	nextIdx := row.NextIdx - 1
	childRow, err := createJob(tx, row.Store, task)
	if err != nil {
		return 0, err
	}
	err = insertJobChild(tx, parentRow, nextIdx, childRow)
	return nextIdx, err
}

func insertJobChild(tx *sqlx.Tx, parentRow int64, idx wantjob.Idx, childRow int64) error {
	_, err := tx.Exec(`INSERT INTO job_children (parent, idx, child) VALUES (?, ?, ?)`, parentRow, idx, childRow)
	return err
}

func GetJobStoreID(tx *sqlx.Tx, jobid wantjob.JobID) (StoreID, error) {
	rowid, err := lookupJobRowID(tx, jobid)
	if err != nil {
		return 0, err
	}
	return dbutil.GetTx[StoreID](tx, `SELECT store_id FROM jobs WHERE rowid = ?`, rowid)
}

func FinishJob(tx *sqlx.Tx, jobid wantjob.JobID, res wantjob.Result) error {
	// TODO: check if any children are still running
	rowid, err := lookupJobRowID(tx, jobid)
	if err != nil {
		return err
	}
	now := tai64.Now()
	_, err = tx.Exec(`UPDATE jobs
		SET state = 3, errcode = ?, res_data = ?, end_at = ?
		WHERE state != 3 AND rowid = ?`, res.ErrCode, res.Data, now.Marshal(), rowid)
	return err
}

func InspectJob(tx *sqlx.Tx, jobid wantjob.JobID) (*wantjob.Job, error) {
	rowid, err := lookupJobRowID(tx, jobid)
	if err != nil {
		return nil, err
	}
	var row struct {
		State      uint32 `db:"state"`
		StartAt    []byte `db:"start_at"`
		ErrCode    uint32 `db:"errcode"`
		ResultData []byte `db:"res_data"`
		EndAt      []byte `db:"end_at"`
	}
	if err := tx.Get(&row, `SELECT state, res_data, errcode, start_at FROM jobs WHERE rowid = ?`, rowid); err != nil {
		return nil, err
	}

	startAt, err := tai64.ParseN(row.StartAt)
	if err != nil {
		return nil, err
	}
	var result *wantjob.Result
	var endAt *tai64.TAI64N
	if row.State == wantjob.JobState_DONE {
		result = &wantjob.Result{
			ErrCode: wantjob.ErrCode(row.ErrCode),
			Data:    row.ResultData,
		}
		ea, err := tai64.ParseN(row.EndAt)
		if err != nil {
			return nil, err
		}
		endAt = &ea
	}
	job := wantjob.Job{
		// TODO: GetTask
		State:   wantjob.JobState(row.State),
		StartAt: startAt,

		Result: result,
		EndAt:  endAt,
	}
	return &job, nil
}

func CacheRead(tx *sqlx.Tx, taskID cadata.ID) ([]byte, StoreID, error) {
	var row struct {
		Data  []byte  `db:"res_data"`
		Store StoreID `db:"store_id"`
	}
	err := tx.Get(&row, `SELECT res_data, store_id FROM jobs
		WHERE task = ? AND state = 3 AND errcode = 0
		LIMIT 1`, taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	return row.Data, row.Store, err
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
		if errors.Is(err, sql.ErrNoRows) {
			return 0, wantjob.ErrJobNotFound{ID: jobid}
		}
		return 0, err
	}
	jobid = jobid[1:]
	for len(jobid) > 0 {
		idx := jobid[0]
		var err error
		rowid, err = dbutil.GetTx[int64](tx, `SELECT child FROM job_children WHERE parent = ? AND idx = ?`, rowid, idx)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return 0, wantjob.ErrJobNotFound{ID: jobid}
			}
			return 0, err
		}
		jobid = jobid[1:]
	}
	return rowid, nil
}
