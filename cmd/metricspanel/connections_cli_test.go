package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConnectionsCLIReadProbeAndStructuredApply(t *testing.T) {
	t.Setenv("METRICSPANEL_TOKEN", "")
	t.Setenv("METRICSPANEL_USERNAME", "")
	t.Setenv("METRICSPANEL_PASSWORD", "")
	var paths []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if r.URL.Path == "/api/v1/connections/apply" || r.URL.Path == "/api/v1/connections/preview" {
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.NotNil(t, body["setup"])
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"accepted":true}`)
	}))
	defer upstream.Close()
	path := filepath.Join(t.TempDir(), "protected-input.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"setup":{"id":"23300000-0000-4000-8000-000000000008","language":"en","alerts":false,"target":{}},"plan_hash":"fixture"}`), 0600))
	for _, action := range []string{"list", "discover", "preview", "apply"} {
		args := []string{"connections", action, "--server", upstream.URL}
		if action == "preview" || action == "apply" {
			args = append(args, "--file", path)
		}
		require.NoError(t, run(args))
	}
	require.Equal(t, []string{"GET /api/v1/connections", "POST /api/v1/connections/discover", "POST /api/v1/connections/preview", "POST /api/v1/connections/apply"}, paths)
	require.Error(t, run([]string{"connections", "apply", "--server", upstream.URL}))
}
