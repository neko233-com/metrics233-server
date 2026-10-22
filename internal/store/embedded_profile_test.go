package store

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/stretchr/testify/require"
)

// Opt-in reproducible workload, not a fragile speed assertion in ordinary CI.
// Includes flat gauges, counters, oscillating gauges and incompressible jitter.
func TestEmbeddedMillionPointProfile(t *testing.T) {
	if os.Getenv("METRICS233_STORAGE_PROFILE") != "1" {
		t.Skip("set METRICS233_STORAGE_PROFILE=1 for the million-point storage profile")
	}
	ctx := t.Context()
	root := t.TempDir()
	legacy, err := Open(filepath.Join(root, "legacy", "control.db"))
	require.NoError(t, err)
	defer legacy.Close()
	control, err := Open(filepath.Join(root, "native", "control.db"))
	require.NoError(t, err)
	defer control.Close()
	engine, err := OpenEmbedded(ctx, control, filepath.Join(root, "native", "metrics"), 15)
	require.NoError(t, err)
	defer engine.Close()
	start := time.Now().Add(-70 * time.Hour).Truncate(time.Second).UnixMilli()
	var legacyTime, nativeTime time.Duration
	var want sampleDigest
	seed := uint64(233)
	for offset := 0; offset < 8192; offset += 32 {
		batch := make([]model.Sample, 0, 4096)
		for point := offset; point < offset+32; point++ {
			for series := 0; series < 128; series++ {
				seed = seed*6364136223846793005 + 1
				value := 42.0
				switch series % 4 {
				case 1:
					value = float64(point*3 + series)
				case 2:
					value = 50 + 25*math.Sin(float64(point)/71)
				case 3:
					value = float64(seed>>11) / float64(uint64(1)<<53) * 100
				}
				s := model.Sample{Name: []string{"profile_constant", "profile_counter_total", "profile_smooth", "profile_jitter"}[series%4], Labels: map[string]string{"service": "game-logic", "serverId": string(rune('A' + series/4)), "env": "test", "role": "logic", "channel": "profile"}, Timestamp: start + int64(point)*30000, Value: value}
				batch = append(batch, s)
				want.add(s)
			}
		}
		at := time.Now()
		require.NoError(t, legacy.Ingest(ctx, batch))
		legacyTime += time.Since(at)
		at = time.Now()
		require.NoError(t, engine.Ingest(ctx, batch))
		nativeTime += time.Since(at)
	}
	allocated := func(root string) int64 {
		var total int64
		require.NoError(t, filepath.WalkDir(root, func(path string, d os.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if !d.IsDir() {
				info, e := d.Info()
				if e != nil {
					return e
				}
				total += allocatedFileBytes(info)
			}
			return nil
		}))
		return total
	}
	walBytes := allocated(filepath.Join(root, "native", "metrics", "timeseries"))
	before := time.Now()
	require.NoError(t, engine.Compact(ctx))
	compactTime := time.Since(before)
	actual, err := engine.fingerprint(ctx, start)
	require.NoError(t, err)
	require.Equal(t, want, actual)
	query := model.Query{Metric: "profile_smooth", Start: start + 8192*30000 - 3600000, End: start + 8192*30000, Step: 30000, Aggregation: "last"}
	before = time.Now()
	result, err := engine.Query(ctx, query)
	require.NoError(t, err)
	queryTime := time.Since(before)
	require.Len(t, result.Series, 32)
	require.NoError(t, engine.Close())
	before = time.Now()
	engine, err = OpenEmbedded(ctx, control, filepath.Join(root, "native", "metrics"), 15)
	require.NoError(t, err)
	defer engine.Close()
	restartTime := time.Since(before)
	_, err = legacy.DB.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	require.NoError(t, err)
	_, err = legacy.DB.ExecContext(ctx, `VACUUM`)
	require.NoError(t, err)
	report := map[string]any{"samples": want.count, "series": 128, "mix": "25% flat, 25% counter, 25% sine, 25% seeded jitter; 30s interval; 68h", "sqlite_bytes": allocated(filepath.Dir(legacy.Path)), "native_total_bytes": allocated(filepath.Dir(control.Path)), "native_timeseries_before_compact_bytes": walBytes, "native_timeseries_after_compact_bytes": allocated(filepath.Join(root, "native", "metrics", "timeseries")), "sqlite_ingest_seconds": legacyTime.Seconds(), "native_fsync_ingest_seconds": nativeTime.Seconds(), "compact_seconds": compactTime.Seconds(), "query_32_series_1h_seconds": queryTime.Seconds(), "restart_seconds": restartTime.Seconds(), "fingerprint": actual.String()}
	raw, err := json.Marshal(report)
	require.NoError(t, err)
	t.Log(string(raw))
	if path := os.Getenv("METRICS233_PROFILE_OUTPUT"); path != "" {
		require.NoError(t, os.WriteFile(path, raw, 0600))
	}
}
