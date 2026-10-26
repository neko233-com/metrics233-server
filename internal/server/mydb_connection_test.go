package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/neko233-com/metrics233-server/internal/collector"
	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/stretchr/testify/require"
)

func TestMyDBAuthenticatedPreviewApplyScrape(t *testing.T) {
	s, h := connectionTestServer(t)
	exporter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-read-only-mydb-credential" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprintln(w, "mydb_up 1\nmydb_connections_active 2\nmydb_max_connections 100\nmydb_audit_healthy 1\nmydb_authentication_failures_total 0\nmydb_security_require_secure_transport 1")
	}))
	defer exporter.Close()
	input := model.ConnectionSetupInput{ID: "23300000-0000-4000-8000-000000000019", Language: "zh", Alerts: true, Target: model.Target{Name: "mydb-fixture", Kind: "mydb", URL: exporter.URL, Enabled: true, IntervalSeconds: 15, SecureSettings: map[string]string{"bearer_token": "synthetic-read-only-mydb-credential"}}}
	preview := connectionCall(h, "POST", "/api/v1/connections/preview", "synthetic-connection-admin-token", map[string]any{"setup": input}, "")
	require.Equal(t, 200, preview.Code, preview.Body.String())
	require.NotContains(t, preview.Body.String(), "synthetic-read-only-mydb-credential")
	var plan model.ConnectionPlan
	require.NoError(t, json.Unmarshal(preview.Body.Bytes(), &plan))
	require.Equal(t, "mydb", plan.Profile)
	apply := connectionCall(h, "POST", "/api/v1/connections/apply", "synthetic-connection-admin-token", map[string]any{"setup": input, "plan_hash": plan.PlanHash}, "")
	require.Equal(t, 200, apply.Code, apply.Body.String())
	targets, err := s.Store.Targets(context.Background())
	require.NoError(t, err)
	require.Len(t, targets, 1)
	count, err := collector.New(s.Store).Scrape(context.Background(), targets[0])
	require.NoError(t, err)
	require.Equal(t, 6, count)
	data, err := s.Store.Query(context.Background(), model.Query{Metric: "mydb_audit_healthy", Start: time.Now().Add(-time.Minute).UnixMilli(), End: time.Now().UnixMilli() + 1, Step: 1000, Aggregation: "last"})
	require.NoError(t, err)
	require.Len(t, data.Series, 1)
	require.Equal(t, 1.0, data.Series[0].Points[0].Value)
}
