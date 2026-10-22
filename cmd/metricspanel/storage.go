package main

import (
	"context"
	"errors"
	"flag"
	"path/filepath"
	"time"

	"github.com/neko233-com/metrics233-server/internal/store"
)

func storageCommand(args []string) error {
	if len(args) == 0 || args[0] != "migrate" {
		return errors.New("use storage migrate --db PATH --metrics-dir PATH --backup-dir NEW_PATH --source-stopped [--prune-legacy]")
	}
	f := flag.NewFlagSet("storage migrate", flag.ContinueOnError)
	db := f.String("db", "data/metrics.db", "offline SQLite control database")
	dest := f.String("metrics-dir", "", "new compressed metrics directory")
	backup := f.String("backup-dir", "", "new protected backup directory; parent must exist")
	stopped := f.Bool("source-stopped", false, "assert every writer to this workspace is stopped")
	prune := f.Bool("prune-legacy", false, "reclaim legacy rows only after backup and restart fingerprint verification")
	days := f.Int("retention-days", store.DefaultRetentionDays, "default days; saved setting takes precedence")
	timeout := f.Duration("timeout", 30*time.Minute, "offline migration deadline")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if f.NArg() != 0 || !*stopped || *backup == "" || *timeout <= 0 {
		return errors.New("offline migration requires --source-stopped, --backup-dir and a positive timeout")
	}
	if *dest == "" {
		*dest = filepath.Join(filepath.Dir(*db), "metrics")
	}
	s, err := store.Open(*db)
	if err != nil {
		return err
	}
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	result, err := s.MigrateEmbedded(ctx, store.MigrationOptions{Destination: *dest, BackupDirectory: *backup, SourceStopped: *stopped, PruneLegacy: *prune, RetentionDays: *days})
	if err != nil {
		return err
	}
	return output(result)
}
