package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/vectorstore"
	"github.com/prometheus/client_golang/prometheus"
	commonmodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	promstorage "github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/util/compression"
)

const EmbeddedMaxSeries = 100000

type embeddedManifest struct {
	Format                string `json:"format"`
	Version               int    `json:"version"`
	OrganizationID        int64  `json:"organization_id"`
	Migrated              bool   `json:"migrated"`
	Samples               int64  `json:"migrated_samples,omitempty"`
	Fingerprint           string `json:"fingerprint,omitempty"`
	NormalizedLabelSeries int    `json:"normalized_label_series,omitempty"`
}

type Embedded struct {
	db                *tsdb.DB
	vectors           *vectorstore.Store
	path, controlPath string
	manifest          embeddedManifest
	days              atomic.Int64
	closed            atomic.Bool
	life              sync.RWMutex
	writer            chan struct{}
	refs              map[string]promstorage.SeriesRef
	refBytes          int
	headSeries        atomic.Int64
	lastSample        atomic.Int64
	statsMu           sync.Mutex
	statsAt           time.Time
	stats             Stats
	rateMu            sync.Mutex
	rate              [60]struct {
		at    int64
		count int
	}
}

func OpenEmbedded(ctx context.Context, control *Store, path string, days int) (*Embedded, error) {
	return openEmbedded(ctx, control, path, days, false)
}

func openEmbedded(ctx context.Context, control *Store, path string, days int, allowLegacy bool) (*Embedded, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if days < 1 || days > 3650 || control == nil {
		return nil, errors.New("invalid embedded storage configuration")
	}
	if !allowLegacy {
		var legacy bool
		if err := control.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM samples LIMIT 1) OR EXISTS(SELECT 1 FROM patterns LIMIT 1)`).Scan(&legacy); err != nil {
			return nil, err
		}
		if legacy {
			var previous embeddedManifest
			raw, err := os.ReadFile(filepath.Join(path, "FORMAT.json"))
			if err != nil || json.Unmarshal(raw, &previous) != nil || !previous.Migrated {
				return nil, errors.New("legacy metrics require an offline storage migrate before selecting embedded storage")
			}
		}
	}
	if info, err := os.Lstat(path); err == nil && !info.IsDir() {
		return nil, errors.New("embedded storage path must be a real directory")
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return nil, err
	}
	manifest := embeddedManifest{Format: "metrics233-embedded", Version: 1, OrganizationID: control.OrganizationID()}
	raw, err := os.ReadFile(filepath.Join(path, "FORMAT.json"))
	if err == nil {
		if json.Unmarshal(raw, &manifest) != nil || manifest.Format != "metrics233-embedded" || manifest.Version != 1 || manifest.OrganizationID != control.OrganizationID() {
			return nil, errors.New("embedded database owner or format mismatch")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	} else {
		entries, e := os.ReadDir(path)
		if e != nil {
			return nil, e
		}
		if len(entries) != 0 {
			return nil, errors.New("refusing to adopt an unowned embedded database")
		}
		if err = writeEmbeddedManifest(path, manifest); err != nil {
			return nil, err
		}
	}
	b := &Embedded{path: path, controlPath: control.Path, manifest: manifest, refs: map[string]promstorage.SeriesRef{}, writer: make(chan struct{}, 1)}
	b.days.Store(int64(days))
	opts := tsdb.DefaultOptions()
	opts.WALCompression = compression.Zstd
	opts.WALSegmentSize = 4 << 20
	opts.RetentionDuration = int64(days) * int64(24*time.Hour/time.Millisecond)
	opts.OutOfOrderTimeWindow = opts.RetentionDuration + int64(5*time.Minute/time.Millisecond)
	opts.MinBlockDuration = int64(2 * time.Hour / time.Millisecond)
	opts.MaxBlockDuration = min(int64(24*time.Hour/time.Millisecond), max(opts.MinBlockDuration, opts.RetentionDuration/10))
	opts.StripeSize = 1024
	opts.WALReplayConcurrency = 2
	opts.HeadChunksWriteBufferSize = 256 << 10
	opts.EnableMemorySnapshotOnShutdown = true
	opts.SeriesLifecycleCallback = b
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	b.db, err = tsdb.Open(filepath.Join(path, "timeseries"), logger, prometheus.NewRegistry(), opts, nil)
	if err != nil {
		return nil, fmt.Errorf("open compressed time series: %w", err)
	}
	b.vectors, err = vectorstore.Open(filepath.Join(path, "vectors"), vectorstore.DefaultMaxVectors)
	if err != nil {
		b.db.Close()
		return nil, err
	}
	last := b.db.Head().MaxTime()
	for _, meta := range b.db.BlockMetas() {
		last = max(last, meta.MaxTime-1)
	}
	if last > 0 {
		b.lastSample.Store(last)
	}
	return b, nil
}

func writeEmbeddedManifest(path string, m embeddedManifest) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	tmp := filepath.Join(path, "FORMAT.json.tmp")
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(raw)
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(path, "FORMAT.json")); err != nil {
		return err
	}
	return syncStorageDirectory(path)
}

func (b *Embedded) Kind() string { return "embedded" }
func (b *Embedded) PreCreation(labels.Labels) error {
	if b.headSeries.Load() >= EmbeddedMaxSeries {
		return errors.New("embedded active-series budget reached")
	}
	return nil
}
func (b *Embedded) PostCreation(labels.Labels) { b.headSeries.Add(1) }
func (b *Embedded) PostDeletion(deleted map[chunks.HeadSeriesRef]labels.Labels) {
	b.headSeries.Add(-int64(len(deleted)))
}
func (b *Embedded) Ping(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if b.closed.Load() {
		return errors.New("embedded storage is closed")
	}
	return nil
}
func (b *Embedded) Close() error {
	b.life.Lock()
	defer b.life.Unlock()
	if b.closed.Swap(true) {
		return nil
	}
	return errors.Join(b.db.Close(), b.vectors.Close())
}

func (b *Embedded) Ingest(ctx context.Context, samples []model.Sample) error {
	if len(samples) < 1 || len(samples) > 10000 {
		return errors.New("batch must contain 1–10000 samples")
	}
	now := time.Now().UnixMilli()
	cutoff := now - b.days.Load()*int64(24*time.Hour/time.Millisecond)
	for _, sample := range samples {
		if err := sample.Validate(); err != nil {
			return err
		}
		if _, ok := sample.Labels[labels.MetricName]; ok {
			return errors.New("omit the reserved __name__ label from native samples")
		}
		if sample.Timestamp != 0 && sample.Timestamp < cutoff {
			return errors.New("sample is older than the configured retention period")
		}
	}
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
	app := b.db.Appender(ctx)
	committed := false
	defer func() {
		if !committed {
			_ = app.Rollback()
		}
	}()
	updates := make(map[string]promstorage.SeriesRef)
	last := int64(0)
	for _, sample := range samples {
		if err := ctx.Err(); err != nil {
			return err
		}
		at := sample.Timestamp
		if at == 0 {
			at = now
		}
		last = max(last, at)
		attrs := canonicalMetricLabels(sample.Labels)
		key := sample.Name + "\x00" + model.LabelsJSON(attrs)
		ref := updates[key]
		if ref == 0 {
			ref = b.refs[key]
		}
		builder := labels.NewScratchBuilder(len(attrs) + 1)
		builder.Add(labels.MetricName, sample.Name)
		for k, v := range attrs {
			builder.Add(k, v)
		}
		builder.Sort()
		ref, err := app.Append(ref, builder.Labels(), at, sample.Value)
		if err != nil {
			return fmt.Errorf("time-series batch rejected: %w", err)
		}
		updates[key] = ref
	}
	if err := app.Commit(); err != nil {
		return err
	}
	committed = true
	// Upstream commits flush complete WAL records to the OS. Sync the current
	// segment before acknowledging, including out-of-order records. Rotated
	// segments are synchronized by upstream. A failed sync remains uncertain.
	if err := b.syncLogs(); err != nil {
		return errors.New("time-series WAL durability could not be confirmed")
	}
	for key, ref := range updates {
		if _, exists := b.refs[key]; !exists {
			b.refBytes += len(key)
		}
		b.refs[key] = ref
	}
	if len(b.refs) > 65536 || b.refBytes > 8<<20 {
		b.refs = map[string]promstorage.SeriesRef{}
		b.refBytes = 0
	}
	for previous := b.lastSample.Load(); last > previous && !b.lastSample.CompareAndSwap(previous, last); previous = b.lastSample.Load() {
	}
	b.rateMu.Lock()
	sec := time.Now().Unix()
	slot := &b.rate[sec%60]
	if slot.at != sec {
		slot.at, slot.count = sec, 0
	}
	slot.count += len(samples)
	b.rateMu.Unlock()
	return nil
}

// Prometheus treats an empty label value as an absent label. Copy only when
// normalization is needed; caller-owned maps may be shared by reporting code.
func canonicalMetricLabels(attrs map[string]string) map[string]string {
	for _, value := range attrs {
		if value == "" {
			out := make(map[string]string, len(attrs))
			for key, value := range attrs {
				if value != "" {
					out[key] = value
				}
			}
			return out
		}
	}
	return attrs
}

func (b *Embedded) syncLogs() error {
	for _, name := range []string{"wal", "wbl"} {
		dir := filepath.Join(b.path, "timeseries", name)
		entries, err := os.ReadDir(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		names := []string{}
		for _, entry := range entries {
			if !entry.IsDir() {
				if _, err := strconv.ParseUint(entry.Name(), 10, 64); err == nil {
					names = append(names, entry.Name())
				}
			}
		}
		sort.Strings(names)
		if len(names) == 0 {
			continue
		}
		f, err := os.OpenFile(filepath.Join(dir, names[len(names)-1]), os.O_RDWR, 0)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		err = errors.Join(f.Sync(), f.Close())
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *Embedded) SetRetention(days int) error {
	return b.setRetention(context.Background(), days)
}

func (b *Embedded) setRetention(ctx context.Context, days int) error {
	if days < 1 || days > 3650 {
		return errors.New("invalid retention days")
	}
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
	cfg := &config.Config{StorageConfig: config.StorageConfig{TSDBConfig: &config.TSDBConfig{OutOfOrderTimeWindow: int64(time.Duration(days)*24*time.Hour/time.Millisecond) + int64(5*time.Minute/time.Millisecond), Retention: &config.TSDBRetentionConfig{Time: commonmodel.Duration(time.Duration(days) * 24 * time.Hour)}}}}
	if err := b.db.ApplyConfig(cfg); err != nil {
		return err
	}
	b.days.Store(int64(days))
	return nil
}

func (b *Embedded) Prune(ctx context.Context, before int64) error {
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
	if before <= 0 {
		return nil
	}
	all := labels.MustNewMatcher(labels.MatchNotEqual, labels.MetricName, "")
	if err := b.db.Delete(ctx, math.MinInt64, before-1, all); err != nil {
		return err
	}
	if err := b.db.CleanTombstones(); err != nil {
		return err
	}
	if b.db.Head().MaxTime() < before {
		if err := b.db.Head().Truncate(before); err != nil {
			return err
		}
	}
	if err := b.syncLogs(); err != nil {
		return err
	}
	if err := b.vectors.Prune(ctx, before); err != nil {
		return err
	}
	b.refs = map[string]promstorage.SeriesRef{}
	b.refBytes = 0
	b.statsMu.Lock()
	b.statsAt = time.Time{}
	b.statsMu.Unlock()
	return nil
}

func (b *Embedded) SavePatterns(ctx context.Context, p []model.Pattern) error {
	return b.vectors.Save(ctx, p)
}
func (b *Embedded) Patterns(ctx context.Context, n int) ([]model.Pattern, error) {
	return b.vectors.List(ctx, n)
}
func (b *Embedded) Pattern(ctx context.Context, id string) (model.Pattern, error) {
	return b.vectors.Get(ctx, id)
}
func (b *Embedded) DeletePattern(ctx context.Context, id string) error {
	return b.vectors.Delete(ctx, id)
}
func (b *Embedded) SearchPatterns(ctx context.Context, q model.PatternSearch) ([]model.PatternHit, error) {
	return b.vectors.Search(ctx, q)
}
