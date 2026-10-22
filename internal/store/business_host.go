package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"github.com/neko233-com/metrics233-server/internal/model"
)

func (s *Store) BusinessHostBinding(ctx context.Context, instanceID string) (model.BusinessHostBinding, error) {
	var b model.BusinessHostBinding
	err := s.DB.QueryRowContext(ctx, `SELECT version,kind,resource_id FROM business_host_bindings WHERE instance_id=?`, instanceID).Scan(&b.Version, &b.Kind, &b.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return b, nil
	}
	return b, err
}

func (s *Store) BindBusinessHost(ctx context.Context, instanceID string, b model.BusinessHostBinding) (model.BusinessHostBinding, error) {
	if b.Version < 0 || len(b.ID) > 128 || (b.Kind == "") != (b.ID == "") || (b.Kind != "" && b.Kind != "managed_host" && b.Kind != "collector") {
		return b, errors.New("invalid host binding")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return b, err
	}
	defer tx.Rollback()
	var exists int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM business_instances WHERE id=?`, instanceID).Scan(&exists); err != nil {
		return b, err
	}
	if exists != 1 {
		return b, sql.ErrNoRows
	}
	if b.Kind != "" {
		query, id := `SELECT count(*) FROM managed_hosts WHERE id=?`, any(b.ID)
		if b.Kind == "collector" {
			n, e := strconv.ParseInt(b.ID, 10, 64)
			if e != nil || n <= 0 {
				return b, errors.New("invalid collector id")
			}
			query, id = `SELECT count(*) FROM targets t LEFT JOIN target_connections c ON c.id=t.id WHERE t.id=? AND COALESCE(c.kind,'') IN ('','prometheus')`, n
		}
		if err = tx.QueryRowContext(ctx, query, id).Scan(&exists); err != nil {
			return b, err
		}
		if exists != 1 {
			return b, sql.ErrNoRows
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO business_host_bindings(instance_id,version,kind,resource_id) VALUES(?,0,'','') ON CONFLICT(instance_id) DO NOTHING`, instanceID)
	if err != nil {
		return b, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE business_host_bindings SET version=version+1,kind=?,resource_id=? WHERE instance_id=? AND version=?`, b.Kind, b.ID, instanceID, b.Version)
	if err != nil {
		return b, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return b, err
	}
	if n != 1 {
		return b, ErrBusinessBindingConflict
	}
	if err = tx.Commit(); err == nil {
		b.Version++
	}
	return b, err
}
