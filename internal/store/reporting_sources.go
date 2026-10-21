package store

import (
	"context"
	"database/sql"
	"errors"
	"net/netip"
	"time"
)

const ReportingSourceLimit = 1024
const ReportingPolicyLimit = 4096

var ErrReportingPolicyConflict = errors.New("reporting policy changed; reload before saving")

type ReportingSource struct {
	Address         string   `json:"address"`
	Disposition     string   `json:"disposition"`
	Allowed         bool     `json:"allowed"`
	FirstSeen       int64    `json:"first_seen"`
	LastSeen        int64    `json:"last_seen"`
	AcceptedBatches int64    `json:"accepted_batches"`
	AcceptedSamples int64    `json:"accepted_samples"`
	Protocols       []string `json:"protocols"`
	LastService     string   `json:"last_service"`
}

type ReportingSources struct {
	Version          int64             `json:"version"`
	AllowlistOnly    bool              `json:"allowlist_only"`
	ObservationLimit int               `json:"observation_limit"`
	PolicyLimit      int               `json:"policy_limit"`
	Sources          []ReportingSource `json:"sources"`
}

type ReportingPolicyUpdate struct {
	Version       int64  `json:"version"`
	AllowlistOnly bool   `json:"allowlist_only"`
	Address       string `json:"address,omitempty"`
	Action        string `json:"action,omitempty"`
}

func CanonicalReportingAddress(raw string) (string, error) {
	ip, err := netip.ParseAddr(raw)
	if err != nil || ip.Zone() != "" || ip.IsUnspecified() || ip.IsMulticast() {
		return "", errors.New("source address must be a literal unicast IP without a port or zone")
	}
	return ip.Unmap().String(), nil
}

func (s *Store) initializeReportingSources() error {
	_, err := s.DB.Exec(`CREATE TABLE IF NOT EXISTS metric_reporting_settings(id INTEGER PRIMARY KEY CHECK(id=1),version INTEGER NOT NULL,allowlist_only INTEGER NOT NULL);
INSERT OR IGNORE INTO metric_reporting_settings(id,version,allowlist_only) VALUES(1,1,0);
CREATE TABLE IF NOT EXISTS metric_reporting_policy(address TEXT PRIMARY KEY,disposition TEXT NOT NULL CHECK(disposition IN ('allow','block')));
CREATE TABLE IF NOT EXISTS metric_reporting_observations(address TEXT PRIMARY KEY,first_seen INTEGER NOT NULL,last_seen INTEGER NOT NULL,accepted_batches INTEGER NOT NULL,accepted_samples INTEGER NOT NULL,protocol_mask INTEGER NOT NULL,last_service TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS metric_reporting_recent ON metric_reporting_observations(last_seen);`)
	return err
}

func (s *Store) ReportingSourceAllowed(ctx context.Context, address string) (bool, error) {
	var restricted bool
	var disposition string
	err := s.DB.QueryRowContext(ctx, `SELECT allowlist_only,COALESCE((SELECT disposition FROM metric_reporting_policy WHERE address=?),'') FROM metric_reporting_settings WHERE id=1`, address).Scan(&restricted, &disposition)
	return err == nil && disposition != "block" && (!restricted || disposition == "allow"), err
}

func (s *Store) RecordReportingSource(ctx context.Context, address, protocol, service string, samples int) error {
	mask := map[string]int{"json": 1, "remote_write": 2, "otlp": 4}[protocol]
	if samples <= 0 || mask == 0 {
		return nil
	}
	if canonical, err := CanonicalReportingAddress(address); err != nil || canonical != address {
		return errors.New("invalid reporting address")
	}
	if len(service) > 128 {
		service = ""
	}
	now := time.Now().UnixMilli()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO metric_reporting_observations(address,first_seen,last_seen,accepted_batches,accepted_samples,protocol_mask,last_service) VALUES(?,?,?,1,?,?,?)`, address, now, now, samples, mask, service)
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if inserted == 0 {
		_, err = tx.ExecContext(ctx, `UPDATE metric_reporting_observations SET last_seen=MAX(last_seen,?),accepted_batches=accepted_batches+1,accepted_samples=accepted_samples+?,protocol_mask=protocol_mask|?,last_service=CASE WHEN ?='' THEN last_service ELSE ? END WHERE address=?`, now, samples, mask, service, service, address)
	} else {
		_, err = tx.ExecContext(ctx, `DELETE FROM metric_reporting_observations WHERE address IN (SELECT address FROM metric_reporting_observations ORDER BY last_seen DESC,address DESC LIMIT -1 OFFSET ?)`, ReportingSourceLimit)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ReportingSources(ctx context.Context) (ReportingSources, error) {
	result := ReportingSources{ObservationLimit: ReportingSourceLimit, PolicyLimit: ReportingPolicyLimit, Sources: []ReportingSource{}}
	// One read transaction keeps the policy version and all row decisions consistent.
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, `SELECT version,allowlist_only FROM metric_reporting_settings WHERE id=1`).Scan(&result.Version, &result.AllowlistOnly); err != nil {
		return result, err
	}
	rows, err := tx.QueryContext(ctx, `WITH addresses AS (SELECT address FROM metric_reporting_observations UNION SELECT address FROM metric_reporting_policy) SELECT a.address,COALESCE(p.disposition,''),COALESCE(o.first_seen,0),COALESCE(o.last_seen,0),COALESCE(o.accepted_batches,0),COALESCE(o.accepted_samples,0),COALESCE(o.protocol_mask,0),COALESCE(o.last_service,'') FROM addresses a LEFT JOIN metric_reporting_observations o ON a.address=o.address LEFT JOIN metric_reporting_policy p ON a.address=p.address ORDER BY COALESCE(o.last_seen,0) DESC,a.address`)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item ReportingSource
		var mask int
		if err = rows.Scan(&item.Address, &item.Disposition, &item.FirstSeen, &item.LastSeen, &item.AcceptedBatches, &item.AcceptedSamples, &mask, &item.LastService); err != nil {
			rows.Close()
			return result, err
		}
		item.Allowed = item.Disposition != "block" && (!result.AllowlistOnly || item.Disposition == "allow")
		item.Protocols = []string{}
		for i, name := range []string{"json", "remote_write", "otlp"} {
			if mask&(1<<i) != 0 {
				item.Protocols = append(item.Protocols, name)
			}
		}
		result.Sources = append(result.Sources, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	return result, tx.Commit()
}

func (s *Store) UpdateReportingPolicy(ctx context.Context, update ReportingPolicyUpdate) error {
	if update.Version < 1 {
		return ErrReportingPolicyConflict
	}
	if update.Address != "" || update.Action != "" {
		var err error
		update.Address, err = CanonicalReportingAddress(update.Address)
		if err != nil {
			return err
		}
		if update.Action != "allow" && update.Action != "block" && update.Action != "reset" {
			return errors.New("source action must be allow, block or reset")
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE metric_reporting_settings SET version=version+1,allowlist_only=? WHERE id=1 AND version=?`, update.AllowlistOnly, update.Version)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrReportingPolicyConflict
	}
	if update.Address != "" {
		if update.Action == "reset" {
			_, err = tx.ExecContext(ctx, `DELETE FROM metric_reporting_policy WHERE address=?`, update.Address)
		} else {
			var count int
			if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM metric_reporting_policy WHERE address<>?`, update.Address).Scan(&count); err != nil {
				return err
			}
			if count >= ReportingPolicyLimit {
				return errors.New("reporting source policy capacity reached")
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO metric_reporting_policy(address,disposition) VALUES(?,?) ON CONFLICT(address) DO UPDATE SET disposition=excluded.disposition`, update.Address, update.Action)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
