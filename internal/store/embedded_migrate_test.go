package store

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedMigrationBackupReopenVerifyAndReclaim(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	path := filepath.Join(root, "control.db")
	s, err := Open(path)
	require.NoError(t, err)
	defer s.Close()
	_, err = s.SaveMetricRetention(ctx, MetricRetention{Days: 3})
	require.NoError(t, err)
	start := time.Now().Add(-time.Hour).UnixMilli()
	var want sampleDigest
	for i := 0; i < 80; i++ {
		point := model.Sample{Name: "cpu_usage", Labels: map[string]string{"serverId": "one"}, Timestamp: start + int64(i)*1000, Value: float64(i) * 1.0000000000000002}
		require.NoError(t, s.Ingest(ctx, []model.Sample{point}))
		want.add(point)
	}
	require.NoError(t, s.Ingest(ctx, []model.Sample{{Name: "expired", Timestamp: time.Now().Add(-4 * 24 * time.Hour).UnixMilli(), Value: 1}}))
	p := model.Pattern{Metric: "cpu_usage", Labels: map[string]string{"serverId": "one"}, Start: start, End: start + 60000, Normalization: "raw", Aggregation: "last", Coverage: 1, Values: make([]float32, 64), CreatedAt: start}
	require.NoError(t, s.SavePatterns(ctx, []model.Pattern{p}))
	p.SetID()
	dest, backup := filepath.Join(root, "metrics"), filepath.Join(root, "backup")
	_, err = s.MigrateEmbedded(ctx, MigrationOptions{Destination: dest, BackupDirectory: backup})
	require.Error(t, err)
	r, err := s.MigrateEmbedded(ctx, MigrationOptions{Destination: dest, BackupDirectory: backup, SourceStopped: true, PruneLegacy: true, RetentionDays: 15})
	require.NoError(t, err)
	require.Equal(t, int64(80), r.Samples)
	require.Equal(t, 1, r.Patterns)
	require.Equal(t, want.String(), r.Fingerprint)
	require.True(t, r.LegacyReclaimed)
	var count int
	require.NoError(t, s.DB.QueryRow(`SELECT count(*) FROM samples`).Scan(&count))
	require.Zero(t, count)
	restored, err := Open(filepath.Join(backup, "control.db"))
	require.NoError(t, err)
	defer restored.Close()
	require.NoError(t, restored.DB.QueryRow(`SELECT count(*) FROM samples`).Scan(&count))
	require.Equal(t, 81, count)
	old, err := restored.Pattern(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, p.Values, old.Values)
	engine, err := OpenEmbedded(ctx, s, dest, 3)
	require.NoError(t, err)
	defer engine.Close()
	actual, err := engine.fingerprint(ctx, 0)
	require.NoError(t, err)
	require.Equal(t, want, actual)
	after, err := engine.Pattern(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, old, after)
	setting, err := s.MetricRetention(ctx, 15)
	require.NoError(t, err)
	require.Equal(t, 3, setting.Days)
	_, err = s.MigrateEmbedded(ctx, MigrationOptions{Destination: dest, BackupDirectory: filepath.Join(root, "second"), SourceStopped: true})
	require.Error(t, err, "must never overwrite a verified destination")
}

// A child exits without any Close/cleanup to exercise acknowledged WAL and LSM
// durability. The parent reopens the actual on-disk files in a fresh process.
func TestEmbeddedAbruptExitDurability(t *testing.T) {
	if root := os.Getenv("METRICS233_CRASH_TEST_ROOT"); root != "" {
		s, err := Open(filepath.Join(root, "control.db"))
		if err != nil {
			os.Exit(10)
		}
		engine, err := OpenEmbedded(context.Background(), s, filepath.Join(root, "metrics"), 15)
		if err != nil {
			os.Exit(11)
		}
		at := time.Now().Add(-time.Minute).Truncate(time.Second).UnixMilli()
		points := []model.Sample{{Name: "crash_proof", Timestamp: at, Value: 17.125}, {Name: "crash_proof", Timestamp: at + 1000, Value: 18.5}}
		if engine.Ingest(context.Background(), points) != nil {
			os.Exit(12)
		}
		p := model.Pattern{Metric: "crash_proof", Start: at, End: at + 60000, Normalization: "raw", Aggregation: "last", Coverage: 1, Values: make([]float32, 64)}
		if engine.SavePatterns(context.Background(), []model.Pattern{p}) != nil {
			os.Exit(13)
		}
		os.Exit(0)
	}
	root := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestEmbeddedAbruptExitDurability$")
	cmd.Env = append(os.Environ(), "METRICS233_CRASH_TEST_ROOT="+root)
	raw, err := cmd.CombinedOutput()
	require.NoError(t, err, string(raw))
	s, err := Open(filepath.Join(root, "control.db"))
	require.NoError(t, err)
	defer s.Close()
	engine, err := OpenEmbedded(t.Context(), s, filepath.Join(root, "metrics"), 15)
	require.NoError(t, err)
	defer engine.Close()
	rows, err := engine.LoadSeries(t.Context(), 0, time.Now().UnixMilli(), nil)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Len(t, rows[0].Points, 2)
	require.Equal(t, 18.5, rows[0].Points[1].Value)
	patterns, err := engine.Patterns(t.Context(), 10)
	require.NoError(t, err)
	require.Len(t, patterns, 1)
}
