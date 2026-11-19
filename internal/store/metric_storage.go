package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const DefaultMetricStorage = "embedded"

type ClickHouseStorageOptions struct {
	URL, Database, User, Password string
}

type MetricStorageOptions struct {
	Strategy, ControlPath, MetricsDir string
	RetentionDays                     int
	ClickHouse                        ClickHouseStorageOptions
}

type metricStorageStrategy func(context.Context, *Store, MetricStorageOptions) (MetricBackend, error)

var metricStorageStrategies = map[string]metricStorageStrategy{
	DefaultMetricStorage: openEmbeddedMetricStorage,
	"sqlite":             useSQLiteMetricStorage,
	"clickhouse":         openClickHouseMetricStorage,
}

func MetricStorageStrategies() []string {
	names := make([]string, 0, len(metricStorageStrategies))
	for name := range metricStorageStrategies {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// OpenMetricStorage selects the backend once, before starting reporting or
// queries. The Store owns its lifetime alongside the SQLite control catalog.
func OpenMetricStorage(ctx context.Context, options MetricStorageOptions) (*Store, error) {
	if options.Strategy == "" {
		options.Strategy = DefaultMetricStorage
	}
	openBackend, ok := metricStorageStrategies[options.Strategy]
	if !ok {
		return nil, fmt.Errorf("unknown metrics storage strategy (choose %s)", strings.Join(MetricStorageStrategies(), ", "))
	}
	if options.RetentionDays == 0 {
		options.RetentionDays = DefaultRetentionDays
	}
	if options.RetentionDays < 1 || options.RetentionDays > 3650 {
		return nil, errors.New("metrics storage retention must be 1–3650 days")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	control, err := Open(options.ControlPath)
	if err != nil {
		return nil, err
	}
	backend, err := openBackend(ctx, control, options)
	if err != nil {
		return nil, errors.Join(err, control.Close())
	}
	control.Backend = backend
	return control, nil
}

func openEmbeddedMetricStorage(ctx context.Context, control *Store, options MetricStorageOptions) (MetricBackend, error) {
	setting, err := control.MetricRetention(ctx, options.RetentionDays)
	if err != nil {
		return nil, err
	}
	dir := options.MetricsDir
	if dir == "" {
		dir = filepath.Join(filepath.Dir(control.Path), "metrics")
	}
	return OpenEmbedded(ctx, control, dir, setting.Days)
}

func useSQLiteMetricStorage(context.Context, *Store, MetricStorageOptions) (MetricBackend, error) {
	// A nil backend routes metrics through Store's existing SQLite implementation.
	return nil, nil
}

func openClickHouseMetricStorage(ctx context.Context, _ *Store, options MetricStorageOptions) (MetricBackend, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	config := options.ClickHouse
	return OpenClickHouse(ctx, config.URL, config.Database, config.User, config.Password, options.RetentionDays)
}
