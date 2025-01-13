package wantdb

import (
	"context"
	"database/sql"
	"errors"
	"sync"

	"github.com/jmoiron/sqlx"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/internal/stores"
)

type StoreID = uint64

type Store struct {
	id StoreID
	tx *sqlx.Tx
	mu sync.Mutex
}

func NewStore(tx *sqlx.Tx, id StoreID) *Store {
	return &Store{tx: tx, id: id}
}

func (s *Store) Post(ctx context.Context, data []byte) (cadata.ID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return PostBlob(s.tx, s.id, data)
}

func (s *Store) Get(ctx context.Context, id cadata.ID, buf []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return GetBlob(s.tx, s.id, id, buf)
}

func (s *Store) Exists(ctx context.Context, id cadata.ID) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return storeContains(s.tx, s.id, id)
}

func (s *Store) Add(ctx context.Context, id cadata.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if exists, err := blobExists(s.tx, id); err != nil {
		return err
	} else if !exists {
		return cadata.ErrNotFound{Key: id}
	}
	return storeAdd(s.tx, s.id, id)
}

func (s *Store) Delete(ctx context.Context, id cadata.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return DeleteBlob(s.tx, s.id, id)
}

func (s *Store) Hash(x []byte) cadata.ID {
	return stores.Hash(x)
}

func (s *Store) MaxSize() int {
	return stores.MaxBlobSize
}

func (s *Store) List(ctx context.Context, span cadata.Span, ids []cadata.ID) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return 0, nil
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

func PostBlob(tx *sqlx.Tx, sid StoreID, data []byte) (cadata.ID, error) {
	if len(data) > stores.MaxBlobSize {
		return cadata.ID{}, cadata.ErrTooLarge
	}
	id := stores.Hash(data)
	yes, err := blobExists(tx, id)
	if err != nil {
		return cadata.ID{}, err
	}
	if !yes {
		if err := insertBlob(tx, id, data); err != nil {
			return cadata.ID{}, err
		}
	}
	if err := storeAdd(tx, sid, id); err != nil {
		return cadata.ID{}, err
	}
	return id, nil
}

func GetBlob(tx *sqlx.Tx, sid StoreID, id cadata.ID, buf []byte) (int, error) {
	var data []byte
	if err := tx.Get(&data, `SELECT blobs.data FROM blobs
		JOIN store_blobs sb ON sb.blob_id = blobs.id
		WHERE store_id = ? AND blob_id = ?`, sid, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, cadata.ErrNotFound{Key: id}
		}
		return 0, err
	}
	return copy(buf, data), nil
}

func DeleteBlob(tx *sqlx.Tx, sid StoreID, id cadata.ID) error {
	if err := storeRemove(tx, sid, id); err != nil {
		return err
	}
	rc, err := getBlobRC(tx, id)
	if err != nil {
		return err
	}
	if rc == 0 {
		return dropBlob(tx, id)
	}
	return err
}

func storeContains(tx *sqlx.Tx, sid StoreID, id cadata.ID) (bool, error) {
	var exists bool
	err := tx.Get(&exists, `SELECT EXISTS (SELECT 1 FROM store_blobs WHERE store_id = ? AND blob_id = ?)`, sid, id)
	return exists, err
}

func storeAdd(tx *sqlx.Tx, sid StoreID, id cadata.ID) error {
	yes, err := storeContains(tx, sid, id)
	if err != nil {
		return err
	}
	if !yes {
		if _, err := tx.Exec(`INSERT INTO store_blobs (store_id, blob_id) VALUES (?, ?)`, sid, id); err != nil {
			return err
		}
		return incrBlobRC(tx, id)
	}
	return nil
}

func storeRemove(tx *sqlx.Tx, sid StoreID, id cadata.ID) error {
	yes, err := storeContains(tx, sid, id)
	if err != nil {
		return err
	}
	if yes {
		if _, err := tx.Exec(`DELETE FROM store_blobs WHERE store_id = ? AND blob_id = ?`, sid, id); err != nil {
			return err
		}
		return decrBlobRC(tx, id)
	}
	return nil
}

// blobExists returns (true, nil) if the blob exists and (false, nil) if it doesn't.
func blobExists(tx *sqlx.Tx, id cadata.ID) (bool, error) {
	var exists bool
	err := tx.Get(&exists, `SELECT EXISTS (SELECT 1 FROM blobs WHERE id = ?)`, id)
	return exists, err
}

// insertBlob inserts an entry into the blobs table
func insertBlob(tx *sqlx.Tx, id cadata.ID, data []byte) error {
	_, err := tx.Exec(`INSERT INTO blobs (id, data, rc) VALUES (?, ?, 0)`, id, data)
	return err
}

func dropBlob(tx *sqlx.Tx, id cadata.ID) error {
	_, err := tx.Exec(`DELETE FROM blobs WHERE id = ?`, id)
	return err
}

func incrBlobRC(tx *sqlx.Tx, id cadata.ID) error {
	_, err := tx.Exec(`UPDATE blobs SET rc = rc + 1 WHERE id = ?`, id)
	return err
}

func decrBlobRC(tx *sqlx.Tx, id cadata.ID) error {
	_, err := tx.Exec(`UPDATE blobs SET rc = rc - 1 WHERE id = ?`, id)
	return err
}

func getBlobRC(tx *sqlx.Tx, id cadata.ID) (int64, error) {
	var rc int64
	err := tx.Get(&rc, `SELECT rc FROM blobs WHERE id = ?`, id)
	return rc, err
}
