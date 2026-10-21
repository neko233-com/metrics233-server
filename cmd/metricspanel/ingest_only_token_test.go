package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServeRejectsInvalidIngestTokenBeforeOpeningStorage(t *testing.T) {
	const admin = "synthetic-management-token-233-0123456789"
	for _, test := range []struct{ mode, token, management string }{
		{"local", "synthetic-ingest-token-233-0123456789", admin},
		{"token", strings.Repeat("x", 31), admin},
		{"token", strings.Repeat("x", 32) + " ", admin},
		{"token", admin, admin},
		{"token", strings.Repeat("x", 32), ""},
	} {
		t.Run(test.mode+strings.Repeat("_", len(test.token)%4), func(t *testing.T) {
			t.Setenv("METRICSPANEL_INGEST_TOKEN", test.token)
			t.Setenv("METRICSPANEL_TOKEN", test.management)
			path := filepath.Join(t.TempDir(), "must-not-create", "control.db")
			err := serve([]string{"--auth", test.mode, "--listen", "127.0.0.1:0", "--db", path})
			require.Error(t, err)
			require.NotContains(t, err.Error(), test.token)
			_, err = os.Stat(filepath.Dir(path))
			require.True(t, os.IsNotExist(err))
		})
	}
}
