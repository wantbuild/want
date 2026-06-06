package wantdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"blobcache.io/blobcache/src/blobcache"
	"github.com/jmoiron/sqlx"
	"go.brendoncarroll.net/state/cadata"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"wantbuild.io/want/src/internal/dbutil"
	"wantbuild.io/want/src/internal/stores"
)

type StoreID = uint64

type DBStore struct {
	id StoreID
	db *sqlx.DB
}

func NewDBStore(db *sqlx.DB, id StoreID) *DBStore {
	return &DBStore{id: id, db: db}
}

func (s *DBStore) Post(ctx context.Context, data []byte) (blobcache.CID, error) {
	return dbutil.DoTx1(ctx, s.db, func(tx *sqlx.Tx) (blobcache.CID, error) {
		return PostBlob(tx, s.id, data)
	})
}

func (s *DBStore) Get(ctx context.Context, id blobcache.CID, buf []byte) (int, error) {
	return dbutil.ROTx1(ctx, s.db, func(tx *sqlx.Tx) (int, error) {
		return GetBlob(tx, 0, id, buf)
	})
}

func (s *DBStore) Exists(ctx context.Context, cids []blobcache.CID, dst *blobcache.BitMap) error {
	return dbutil.ROTx(ctx, s.db, func(tx *sqlx.Tx) error {
		return storeContains(tx, 0, cids, dst)
	})
}

func (s *DBStore) Add(ctx context.Context, id blobcache.CID) error {
	return dbutil.DoTx(ctx, s.db, func(tx *sqlx.Tx) error {
		return AddBlob(tx, s.id, id)
	})
}

func (s *DBStore) Delete(ctx context.Context, cids []blobcache.CID) error {
	return dbutil.DoTx(ctx, s.db, func(tx *sqlx.Tx) error {
		return DeleteBlob(tx, s.id, cids)
	})
}

func (s *DBStore) Hash(x []byte) blobcache.CID {
	return stores.Hash(x)
}

func (s *DBStore) MaxSize() int {
	return stores.MaxBlobSize
}

func (s *DBStore) StoreID() StoreID {
	return s.id
}

func (s *DBStore) Pull(ctx context.Context, root []byte) error {
	return dbutil.DoTx(ctx, s.db, func(tx *sqlx.Tx) error {
		return Pull(tx, s.id, root)
	})
}

func (s *DBStore) String() string {
	return fmt.Sprintf("DBStore(%v)", s.id)
}

type TxStore struct {
	id StoreID
	tx *sqlx.Tx
	mu sync.Mutex
}

func NewTxStore(tx *sqlx.Tx, id StoreID) *TxStore {
	return &TxStore{
		id: id,
		tx: tx,
	}
}

func (s *TxStore) Post(ctx context.Context, data []byte) (blobcache.CID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return PostBlob(s.tx, s.id, data)
}

func (s *TxStore) Get(ctx context.Context, id blobcache.CID, buf []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return GetBlob(s.tx, s.id, id, buf)
}

func (s *TxStore) Exists(ctx context.Context, cids []blobcache.CID, dst *blobcache.BitMap) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return storeContains(s.tx, s.id, cids, dst)
}

func (s *TxStore) Add(ctx context.Context, id blobcache.CID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return AddBlob(s.tx, s.id, id)
}

func (s *TxStore) Delete(ctx context.Context, cids []blobcache.CID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return DeleteBlob(s.tx, s.id, cids)
}

func (s *TxStore) Hash(x []byte) blobcache.CID {
	return stores.Hash(x)
}

func (s *TxStore) MaxSize() int {
	return stores.MaxBlobSize
}

func (s *TxStore) StoreID() StoreID {
	return s.id
}

func (s *TxStore) Pull(ctx context.Context, root []byte) error {
	return Pull(s.tx, s.id, root)
}

func CreateStore(tx *sqlx.Tx) (StoreID, error) {
	var ret StoreID
	err := tx.Get(&ret, `INSERT INTO stores DEFAULT VALUES RETURNING id`)
	return ret, err
}

func DropStore(tx *sqlx.Tx, sid StoreID) error {
	if _, err := tx.Exec(`UPDATE blobs
		SET rc = rc - 1
		WHERE id IN (
			SELECT blob_id FROM store_blobs WHERE store_id = ?
		)`, sid); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM store_blobs WHERE store_id = ?`, sid); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM stores WHERE id = ?`, sid); err != nil {
		return err
	}
	return nil
}

func PostBlob(tx *sqlx.Tx, sid StoreID, data []byte) (blobcache.CID, error) {
	if len(data) > stores.MaxBlobSize {
		return blobcache.CID{}, cadata.ErrTooLarge
	}
	id := stores.Hash(data)
	yes, err := blobExists(tx, id)
	if err != nil {
		return blobcache.CID{}, err
	}
	if !yes {
		if err := insertBlob(tx, id, data); err != nil {
			return blobcache.CID{}, err
		}
	}
	if err := AddBlob(tx, sid, id); err != nil {
		return blobcache.CID{}, err
	}
	return id, nil
}

func GetBlob(tx *sqlx.Tx, sid StoreID, id blobcache.CID, buf []byte) (int, error) {
	var data []byte
	var err error
	if sid == 0 {
		err = tx.Get(&data, `SELECT blobs.data FROM blobs WHERE id = ?`, id)
	} else {
		err = tx.Get(&data, `SELECT blobs.data FROM blobs
		JOIN store_blobs sb ON sb.blob_id = blobs.id
		WHERE store_id = ? AND blob_id = ?`, sid, id)
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, blobcache.ErrNotFound{CID: id}
		}
		return 0, err
	}
	if stores.Hash(data) != id {
		return 0, fmt.Errorf("db returned bad data for %v", id)
	}
	return copy(buf, data), nil
}

func AddBlob(tx *sqlx.Tx, sid StoreID, id blobcache.CID) error {
	if _, err := tx.Exec(`INSERT OR IGNORE INTO store_blobs (store_id, blob_id) VALUES (?, ?)`, sid, id); err != nil {
		sqlerr := &sqlite.Error{}
		if errors.As(err, &sqlerr) {
			if sqlerr.Code() == sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY {
				return blobcache.ErrNotFound{CID: id}
			}
		}
		return err
	}
	if _, err := tx.Exec(`UPDATE blobs SET rc = rc + CHANGES() WHERE id = ?`, id); err != nil {
		return err
	}
	return nil
}

func DeleteBlob(tx *sqlx.Tx, sid StoreID, cids []blobcache.CID) error {
	if err := storeRemove(tx, sid, cids); err != nil {
		return err
	}
	for _, cid := range cids {
		rc, err := getBlobRC(tx, cid)
		if err != nil {
			return err
		}
		if rc == 0 {
			return dropBlob(tx, cid)
		}
	}
	return nil
}

func CopyAll(tx *sqlx.Tx, src, dst StoreID) error {
	_, err := tx.Exec(`INSERT OR IGNORE INTO store_blobs (store_id, blob_id)
		SELECT ? as store_id, blob_id
		FROM store_blobs
		WHERE store_id = ?
	`, dst, src)
	return err
}

func storeContains(tx *sqlx.Tx, sid StoreID, cids []blobcache.CID, dst *blobcache.BitMap) error {
	var err error
	var exists bool
	if sid == 0 {
		for i, cid := range cids {
			if err = tx.Get(&exists, `SELECT EXISTS (SELECT 1 FROM blobs WHERE id = ?)`, cid); err != nil {
				return err
			}
			if exists {
				dst.Set(i)
			}
		}
	} else {
		for i, cid := range cids {
			if err = tx.Get(&exists, `SELECT EXISTS (SELECT 1 FROM store_blobs WHERE store_id = ? AND blob_id = ?)`, sid, cid); err != nil {
				return err
			}
			if exists {
				dst.Set(i)
			}
		}
	}
	return nil
}

func storeRemove(tx *sqlx.Tx, sid StoreID, cids []blobcache.CID) error {
	var exists blobcache.BitMap
	if err := storeContains(tx, sid, cids, &exists); err != nil {
		return err
	}
	for i := range cids {
		yes := exists.IsSet(i)
		if !yes {
			continue
		}
		cid := cids[i]
		if _, err := tx.Exec(`DELETE FROM store_blobs WHERE store_id = ? AND blob_id = ?`, sid, cid); err != nil {
			return err
		}
		return decrBlobRC(tx, cid)
	}
	return nil
}

// blobExists returns (true, nil) if the blob exists and (false, nil) if it doesn't.
func blobExists(tx *sqlx.Tx, id blobcache.CID) (bool, error) {
	var exists bool
	err := tx.Get(&exists, `SELECT EXISTS (SELECT 1 FROM blobs WHERE id = ?)`, id)
	return exists, err
}

// insertBlob inserts an entry into the blobs table
func insertBlob(tx *sqlx.Tx, id blobcache.CID, data []byte) error {
	_, err := tx.Exec(`INSERT INTO blobs (id, data, rc) VALUES (?, ?, 0)`, id, data)
	return err
}

func dropBlob(tx *sqlx.Tx, id blobcache.CID) error {
	_, err := tx.Exec(`DELETE FROM blobs WHERE id = ?`, id)
	return err
}

func decrBlobRC(tx *sqlx.Tx, id blobcache.CID) error {
	_, err := tx.Exec(`UPDATE blobs SET rc = rc - 1 WHERE id = ?`, id)
	return err
}

func getBlobRC(tx *sqlx.Tx, id blobcache.CID) (int64, error) {
	var rc int64
	err := tx.Get(&rc, `SELECT rc FROM blobs WHERE id = ?`, id)
	return rc, err
}
