package server

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/stretchr/testify/require"
)

func TestAgentDiscoveryUsesActualRegistrationsAndProtectsManagement(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "control.db"))
	require.NoError(t, err)
	defer s.DB.Close()
	srv := New(s, "synthetic-management-credential", 15)
	srv.ingestToken = "synthetic-ingest-credential-000000000000"
	h, manifest := srv.handlerWithRouteManifest()
	call := func(token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/api/v1/agent/capabilities", nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	require.Equal(t, 401, call("").Code)
	require.Equal(t, 403, call(srv.ingestToken).Code)
	w := call(srv.Token)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	var data struct {
		SchemaVersion string           `json:"schema_version"`
		Routes        []ProtectedRoute `json:"protected_route_inventory"`
		Operations    []agentOperation `json:"operations"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &data))
	require.Equal(t, "1", data.SchemaVersion)
	require.Equal(t, manifest, data.Routes)
	registered := map[string]bool{}
	for _, r := range manifest {
		registered[r.Pattern] = true
	}
	for _, op := range data.Operations {
		if op.ID == "metrics.query" {
			continue
		} // Prometheus owns the registered catch-all.
		require.True(t, registered[op.Method+" "+op.Path], "unregistered operation %s", op.ID)
		require.NotEmpty(t, op.Concurrency)
	}
	operations := map[string]agentOperation{}
	for _, op := range data.Operations {
		operations[op.ID] = op
	}
	require.Equal(t, "Admin", operations["business.host-binding"].MinimumRole)
	require.Equal(t, "Admin", operations["business.host-options"].MinimumRole)
	require.Equal(t, "Viewer", operations["business.resources"].MinimumRole)
	require.Equal(t, []any{"version", "kind", "id"}, operations["business.host-binding"].Schema["required"])
	require.Contains(t, operations["business.ranking"].Concurrency, "sort")
	require.NotContains(t, w.Body.String(), srv.Token)
	require.NotContains(t, w.Body.String(), srv.ingestToken)
}
