package collector

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	model "github.com/neko233-com/metrics233-server/internal/telemetry"
	"github.com/stretchr/testify/require"
)

// Opt-in interoperability check against actual binaries. The private fixture
// file is outside source control and is never logged, including on failure.
func TestLiveDatabaseMonitoring(t *testing.T) {
	path := os.Getenv("METRICS233_DATABASE_TEST_FILE")
	if path == "" {
		t.Skip("requires a private live database fixture file")
	}
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var fixtures []struct {
		Target   model.Target `json:"target"`
		Required []string     `json:"required"`
	}
	require.NoError(t, json.Unmarshal(raw, &fixtures))
	require.NotEmpty(t, fixtures)
	for _, fixture := range fixtures {
		t.Run(fixture.Target.Kind, func(t *testing.T) {
			require.NoError(t, fixture.Target.Validate())
			samples, err := New(nil).collect(context.Background(), fixture.Target, fixture.Target.SecureSettings)
			require.NoError(t, err)
			names := map[string]bool{}
			for _, sample := range samples {
				require.NoError(t, sample.Validate())
				names[sample.Name] = true
			}
			for _, name := range fixture.Required {
				require.True(t, names[name], "missing metric %s", name)
			}
		})
	}
}
