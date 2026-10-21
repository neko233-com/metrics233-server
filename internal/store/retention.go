package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const DefaultRetentionDays = 15

var ErrRetentionConflict = errors.New("retention setting changed; reload before saving")

func retentionDefault(days int) int {
	if days < 1 || days > 3650 {
		return DefaultRetentionDays
	}
	return days
}

type MetricRetention struct {
	Days    int   `json:"days"`
	Version int64 `json:"version"`
}

// MetricRetention returns a durable override, or the explicit startup default.
// Reads never initialize a setting or overwrite an operator's saved preference.
func (s *Store) MetricRetention(ctx context.Context, fallback int) (MetricRetention, error) {
	fallback = retentionDefault(fallback)
	value := MetricRetention{Days: fallback}
	if s.Backend != nil {
		return value, nil
	}
	err := s.DB.QueryRowContext(ctx, `SELECT days,version FROM metric_retention WHERE id=1`).Scan(&value.Days, &value.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return value, nil
	}
	return value, err
}

// PruneMetricRetention serializes the cutoff read and deletion with setting
// saves. A successfully extended lifetime cannot be followed by a cleanup
// committing an older, shorter setting read outside its transaction.
func (s *Store) PruneMetricRetention(ctx context.Context, fallback int, now time.Time) error {
	days := retentionDefault(fallback)
	if s.Backend != nil {
		return s.Backend.Prune(ctx, now.Add(-time.Duration(days)*24*time.Hour).UnixMilli())
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, `SELECT days FROM metric_retention WHERE id=1`).Scan(&days)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err := pruneMetricRows(ctx, tx, now.Add(-time.Duration(days)*24*time.Hour).UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SaveMetricRetention(ctx context.Context, value MetricRetention) (MetricRetention, error) {
	if value.Days < 1 || value.Days > 3650 || value.Version < 0 {
		return MetricRetention{}, errors.New("retention days must be an integer between 1 and 3650 with a nonnegative version")
	}
	if s.Backend != nil {
		return MetricRetention{}, errors.New("ClickHouse retention is configured explicitly with its table TTL and startup setting")
	}
	result, err := s.DB.ExecContext(ctx, `INSERT INTO metric_retention(id,days,version)
SELECT 1,?,1 WHERE ?=0 OR EXISTS(SELECT 1 FROM metric_retention WHERE id=1)
ON CONFLICT(id) DO UPDATE SET days=excluded.days,version=metric_retention.version+1 WHERE metric_retention.version=?`, value.Days, value.Version, value.Version)
	if err != nil {
		return MetricRetention{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return MetricRetention{}, err
	}
	if count != 1 {
		return MetricRetention{}, ErrRetentionConflict
	}
	return MetricRetention{Days: value.Days, Version: value.Version + 1}, nil
}
