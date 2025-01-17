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
	"wantbuild.io/want/lib/wantjob"
)

func CreateRootJob(tx *sqlx.Tx, task wantjob.Task) (wantjob.Idx, error) {
	rowid, err := createJob(tx, task)
	if err != nil {
		return 0, err
	}
	// create the root entry
	return dbutil.GetTx[wantjob.Idx](tx, `INSERT INTO job_roots (job_row) VALUES (?) RETURNING idx`, rowid)
}

func CreateChildJob(tx *sqlx.Tx, parentID wantjob.JobID, task wantjob.Task) (wantjob.Idx, error) {
	parentRow, err := lookupJobRowID(tx, parentID)
	if err != nil {
		return 0, err
	}
	var nextIdx wantjob.Idx
	if err := tx.Get(&nextIdx, `UPDATE jobs SET next_idx = next_idx + 1 WHERE rowid = ? RETURNING next_idx`, parentRow); err != nil {
		return 0, err
	}
	nextIdx--
	childRow, err := createJob(tx, task)
	if err != nil {
		return 0, err
	}
	if err := insertJobChild(tx, parentRow, nextIdx, childRow); err != nil {
		return 0, err
	}
	return nextIdx, nil
}

func createJob(tx *sqlx.Tx, task wantjob.Task) (int64, error) {
	taskID, err := ensureTask(tx, task)
	if err != nil {
		return 0, err
	}

	data, sid, err := cacheRead(tx, task.ID())
	if err != nil {
		return 0, err
	}
	cacheHit := len(data) > 0
	if !cacheHit {
		sid, err = CreateStore(tx)
		if err != nil {
			return 0, err
		}
	}

	now := tai64.Now()
	rowid, err := dbutil.GetTx[int64](tx, `INSERT INTO jobs (task, store_id, start_at) VALUES (?, ?, ?) RETURNING rowid`, taskID, sid, now.Marshal())
	if err != nil {
		return 0, err
	}
	if cacheHit {
		if err := finishJobAtRow(tx, rowid, wantjob.Result{Data: data}); err != nil {
			return 0, err
		}
	}
	return rowid, nil
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
	return finishJobAtRow(tx, rowid, res)
}

func finishJobAtRow(tx *sqlx.Tx, rowid int64, res wantjob.Result) error {
	now := tai64.Now()
	_, err := tx.Exec(`UPDATE jobs
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
		TaskID     wantjob.TaskID            `db:"task"`
		State      wantjob.JobState          `db:"state"`
		StartAt    []byte                    `db:"start_at"`
		ErrCode    sql.Null[wantjob.ErrCode] `db:"errcode"`
		ResultData []byte                    `db:"res_data"`
		EndAt      []byte                    `db:"end_at"`
	}
	if err := tx.Get(&row, `SELECT task, state, start_at, errcode, res_data, end_at FROM jobs WHERE rowid = ?`, rowid); err != nil {
		return nil, err
	}

	startAt, err := tai64.ParseN(row.StartAt)
	if err != nil {
		return nil, err
	}
	var result *wantjob.Result
	var endAt *tai64.TAI64N
	if row.State == wantjob.DONE {
		result = &wantjob.Result{
			ErrCode: wantjob.ErrCode(row.ErrCode.V),
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
		State:   row.State,
		StartAt: startAt,

		Result: result,
		EndAt:  endAt,
	}
	return &job, nil
}

func cacheRead(tx *sqlx.Tx, taskID cadata.ID) ([]byte, StoreID, error) {
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
