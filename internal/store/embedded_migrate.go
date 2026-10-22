package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

type MigrationOptions struct {
	Destination, BackupDirectory string
	SourceStopped, PruneLegacy   bool
	RetentionDays                int
}

type MigrationResult struct {
	Samples         int64  `json:"samples"`
	Patterns        int    `json:"patterns"`
	Fingerprint     string `json:"fingerprint"`
	LegacyReclaimed bool   `json:"legacy_reclaimed"`
}

// sampleDigest is an order-independent cryptographic multiset checksum. Count
// and modular addition preserve multiplicity; hashes include exact float bits.
type sampleDigest struct {
	count int64
	sum   [32]byte
}

func (d *sampleDigest) add(s model.Sample) {
	h := sha256.New()
	for _, part := range []string{s.Name, model.LabelsJSON(s.Labels)} {
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(part)))
		h.Write(size[:])
		h.Write([]byte(part))
	}
	var data [16]byte
	binary.BigEndian.PutUint64(data[:8], uint64(s.Timestamp))
	binary.BigEndian.PutUint64(data[8:], math.Float64bits(s.Value))
	h.Write(data[:])
	hash := h.Sum(nil)
	carry := 0
	for i := 31; i >= 0; i-- {
		n := int(d.sum[i]) + int(hash[i]) + carry
		d.sum[i] = byte(n)
		carry = n >> 8
	}
	d.count++
}
func (d sampleDigest) String() string {
	return fmt.Sprintf("%d:%s", d.count, hex.EncodeToString(d.sum[:]))
}

func (b *Embedded) fingerprint(ctx context.Context, cutoff int64) (sampleDigest, error) {
	q, err := b.db.Querier(cutoff, math.MaxInt64)
	if err != nil {
		return sampleDigest{}, err
	}
	defer q.Close()
	set := q.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchNotEqual, labels.MetricName, ""))
	var digest sampleDigest
	for set.Next() {
		series := set.At()
		name, attrs := seriesIdentity(series.Labels())
		it := series.Iterator(nil)
		for kind := it.Next(); kind != chunkenc.ValNone; kind = it.Next() {
			if err := ctx.Err(); err != nil {
				return digest, err
			}
			if kind != chunkenc.ValFloat {
				return digest, errors.New("unexpected non-float migration record")
			}
			at, value := it.At()
			digest.add(model.Sample{Name: name, Labels: attrs, Timestamp: at, Value: value})
		}
		if err := it.Err(); err != nil {
			return digest, err
		}
	}
	return digest, set.Err()
}

// Compact flushes the complete offline head. Live ingestion never calls it.
func (b *Embedded) Compact(ctx context.Context) error {
	b.life.RLock()
	defer b.life.RUnlock()
	if err := b.Ping(ctx); err != nil {
		return err
	}
	select {
	case b.writer <- struct{}{}:
		defer func() { <-b.writer }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if b.db.Head().MinTime() <= b.db.Head().MaxTime() {
		if err := b.db.CompactHead(tsdb.NewRangeHead(b.db.Head(), b.db.Head().MinTime(), b.db.Head().MaxTime())); err != nil {
			return err
		}
	}
	if err := b.db.CompactOOOHead(ctx); err != nil {
		return err
	}
	return b.syncLogs()
}

// MigrateEmbedded only runs offline. It never removes legacy observations until
// a protected SQLite backup and an independently reopened exact copy exist.
func (s *Store) MigrateEmbedded(ctx context.Context, opt MigrationOptions) (result MigrationResult, err error) {
	if !opt.SourceStopped || s.Backend != nil {
		return result, errors.New("migration requires --source-stopped and the offline SQLite control store")
	}
	setting, err := s.MetricRetention(ctx, opt.RetentionDays)
	if err != nil {
		return result, err
	}
	dest, err := filepath.Abs(opt.Destination)
	if err != nil {
		return result, err
	}
	backup, err := filepath.Abs(opt.BackupDirectory)
	if err != nil {
		return result, err
	}
	if opt.Destination == "" || opt.BackupDirectory == "" || dest == backup || filepath.Dir(dest) == dest {
		return result, errors.New("distinct destination and new backup directory are required")
	}
	if _, err = os.Lstat(dest); !errors.Is(err, os.ErrNotExist) {
		return result, errors.New("migration destination must not exist")
	}
	if err = os.Mkdir(backup, 0700); err != nil {
		return result, errors.New("backup directory must be new and its parent must exist")
	}
	if _, err = s.DB.ExecContext(ctx, `VACUUM INTO ?`, filepath.Join(backup, "control.db")); err != nil {
		return result, fmt.Errorf("consistent control backup: %w", err)
	}
	if err = os.Chmod(filepath.Join(backup, "control.db"), 0600); err != nil {
		return result, err
	}
	for _, name := range []string{"secrets.key", "identity.db"} {
		source := filepath.Join(filepath.Dir(s.Path), name)
		info, e := os.Lstat(source)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil || !info.Mode().IsRegular() {
			return result, errors.New("backup source must be a regular file")
		}
		if name == "identity.db" {
			db, e := sql.Open("sqlite", source)
			if e != nil {
				return result, e
			}
			_, e = db.ExecContext(ctx, `VACUUM INTO ?`, filepath.Join(backup, name))
			e = errors.Join(e, db.Close())
			if e != nil {
				return result, e
			}
		} else {
			raw, e := os.ReadFile(source)
			if e != nil {
				return result, e
			}
			if e = os.WriteFile(filepath.Join(backup, name), raw, 0600); e != nil {
				return result, e
			}
		}
		if err = os.Chmod(filepath.Join(backup, name), 0600); err != nil {
			return result, err
		}
	}
	for _, name := range []string{"control.db", "secrets.key", "identity.db"} {
		f, e := os.OpenFile(filepath.Join(backup, name), os.O_RDWR, 0)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return result, e
		}
		e = errors.Join(f.Sync(), f.Close())
		if e != nil {
			return result, e
		}
	}
	if err = syncStorageDirectory(backup); err != nil {
		return result, err
	}
	if err = syncStorageDirectory(filepath.Dir(backup)); err != nil {
		return result, err
	}
	prepared, err := os.MkdirTemp(filepath.Dir(dest), ".metrics-migrate-")
	if err != nil {
		return result, err
	}
	engine, err := openEmbedded(ctx, s, prepared, setting.Days, true)
	if err != nil {
		return result, err
	}
	defer func() {
		if engine != nil {
			err = errors.Join(err, engine.Close())
		}
	}()
	cutoff := time.Now().Add(-time.Duration(setting.Days) * 24 * time.Hour).UnixMilli()
	rows, err := s.DB.QueryContext(ctx, `SELECT s.name,s.labels,m.timestamp,m.value FROM samples m JOIN series s ON s.id=m.series_id WHERE m.timestamp>=? ORDER BY m.timestamp,m.series_id`, cutoff)
	if err != nil {
		return result, err
	}
	var expected sampleDigest
	batch := make([]model.Sample, 0, 5000)
	for rows.Next() {
		var sample model.Sample
		var attrs string
		if err = rows.Scan(&sample.Name, &attrs, &sample.Timestamp, &sample.Value); err != nil {
			break
		}
		if err = json.Unmarshal([]byte(attrs), &sample.Labels); err != nil {
			break
		}
		expected.add(sample)
		batch = append(batch, sample)
		if len(batch) == cap(batch) {
			if err = engine.Ingest(ctx, batch); err != nil {
				break
			}
			batch = batch[:0]
		}
	}
	err = errors.Join(err, rows.Err(), rows.Close())
	if err != nil {
		return result, err
	}
	if len(batch) > 0 {
		if err = engine.Ingest(ctx, batch); err != nil {
			return result, err
		}
	}
	rows, err = s.DB.QueryContext(ctx, `SELECT payload FROM patterns ORDER BY id`)
	if err != nil {
		return result, err
	}
	patternHashes := map[string][32]byte{}
	for rows.Next() {
		var raw []byte
		var p model.Pattern
		if err = rows.Scan(&raw); err != nil {
			break
		}
		if err = json.Unmarshal(raw, &p); err != nil {
			break
		}
		if p.End < cutoff {
			continue
		}
		if err = engine.SavePatterns(ctx, []model.Pattern{p}); err != nil {
			break
		}
		encoded, _ := json.Marshal(p)
		patternHashes[p.ID] = sha256.Sum256(encoded)
	}
	err = errors.Join(err, rows.Err(), rows.Close())
	if err != nil {
		return result, err
	}
	if err = engine.Compact(ctx); err != nil {
		return result, err
	}
	if err = engine.Close(); err != nil {
		return result, err
	}
	engine = nil
	engine, err = openEmbedded(ctx, s, prepared, setting.Days, true)
	if err != nil {
		return result, err
	}
	actual, err := engine.fingerprint(ctx, cutoff)
	if err != nil {
		return result, err
	}
	if actual != expected {
		return result, errors.New("migration sample count or lossless fingerprint mismatch; original data preserved")
	}
	for id, hash := range patternHashes {
		p, e := engine.Pattern(ctx, id)
		if e != nil {
			return result, e
		}
		raw, _ := json.Marshal(p)
		if sha256.Sum256(raw) != hash {
			return result, errors.New("migration vector fingerprint mismatch")
		}
	}
	engine.manifest.Migrated = true
	engine.manifest.Samples = actual.count
	engine.manifest.Fingerprint = actual.String()
	if err = writeEmbeddedManifest(prepared, engine.manifest); err != nil {
		return result, err
	}
	if err = engine.Close(); err != nil {
		return result, err
	}
	engine = nil
	if err = os.Rename(prepared, dest); err != nil {
		return result, err
	}
	if err = syncStorageDirectory(filepath.Dir(dest)); err != nil {
		return result, err
	}
	result = MigrationResult{Samples: actual.count, Patterns: len(patternHashes), Fingerprint: actual.String()}
	if opt.PruneLegacy {
		tx, e := s.DB.BeginTx(ctx, nil)
		if e != nil {
			return result, e
		}
		for _, table := range []string{"samples", "series", "patterns"} {
			if _, e = tx.ExecContext(ctx, `DELETE FROM `+table); e != nil {
				tx.Rollback()
				return result, e
			}
		}
		if e = tx.Commit(); e != nil {
			return result, e
		}
		if _, e = s.DB.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); e != nil {
			return result, e
		}
		if _, e = s.DB.ExecContext(ctx, `VACUUM`); e != nil {
			return result, e
		}
		result.LegacyReclaimed = true
	}
	return result, nil
}
