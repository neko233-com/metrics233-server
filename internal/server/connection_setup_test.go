package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/neko233-com/metrics233-server/internal/identity"
	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/stretchr/testify/require"
)

func connectionTestServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "control.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	s := New(db, "synthetic-connection-admin-token", 15)
	s.ingestToken = "synthetic-connection-ingest-token-000000"
	return s, s.Handler()
}

func connectionCall(h http.Handler, method, path, token string, body any, role identity.Role) *httptest.ResponseRecorder {
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if role != "" {
		r = r.WithContext(identity.WithPrincipal(r.Context(), identity.Principal{OrgID: 1, Type: identity.TypeUser, ID: 1, Role: role, Login: "fixture"}))
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestConnectionSetupPreviewApplyRealHTTPRetryAndRoleIsolation(t *testing.T) {
	s, h := connectionTestServer(t)
	var calls atomic.Int32
	var changed atomic.Bool
	exporter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		user, password, ok := r.BasicAuth()
		if !ok || user != "metrics" || password != "synthetic-http-password" {
			w.WriteHeader(401)
			return
		}
		fmt.Fprintln(w, "fixture_connections 233")
		if changed.Load() {
			fmt.Fprintln(w, "fixture_extra 1")
		}
	}))
	defer exporter.Close()
	input := model.ConnectionSetupInput{ID: "23300000-0000-4000-8000-000000000005", Target: model.Target{Name: "HTTP fixture", Kind: "prometheus", URL: exporter.URL, Username: "metrics", SecureSettings: map[string]string{"password": "synthetic-http-password"}, IntervalSeconds: 15, Enabled: true}, Language: "zh", Alerts: true}
	request := connectionSetupRequest{Setup: input}
	for _, role := range []identity.Role{identity.RoleViewer, identity.RoleEditor} {
		w := connectionCall(h, "POST", "/api/v1/connections/preview", s.Token, request, role)
		require.Equal(t, 403, w.Code, w.Body.String())
	}
	require.Equal(t, 401, connectionCall(h, "POST", "/api/v1/connections/preview", "", request, "").Code)
	require.Equal(t, 403, connectionCall(h, "POST", "/api/v1/connections/preview", s.ingestToken, request, "").Code)
	require.Zero(t, calls.Load())
	w := connectionCall(h, "POST", "/api/v1/connections/preview", s.Token, request, identity.RoleAdmin)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.NotContains(t, w.Body.String(), "synthetic-http-password")
	var plan model.ConnectionPlan
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &plan))
	require.NotEmpty(t, plan.PlanHash)
	metrics, err := s.Store.Metrics(context.Background())
	require.NoError(t, err)
	require.Empty(t, metrics)
	targets, err := s.Store.Targets(context.Background())
	require.NoError(t, err)
	require.Empty(t, targets)
	request.PlanHash = plan.PlanHash
	changed.Store(true)
	w = connectionCall(h, "POST", "/api/v1/connections/apply", s.Token, request, identity.RoleAdmin)
	require.Equal(t, 409, w.Code, w.Body.String())
	targets, err = s.Store.Targets(context.Background())
	require.NoError(t, err)
	require.Empty(t, targets)
	changed.Store(false)
	w = connectionCall(h, "POST", "/api/v1/connections/apply", s.Token, request, identity.RoleAdmin)
	require.Equal(t, 200, w.Code, w.Body.String())
	var receipt model.ConnectionReceipt
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &receipt))
	before := calls.Load()
	exporter.Close()
	w = connectionCall(h, "POST", "/api/v1/connections/apply", s.Token, request, identity.RoleAdmin)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, before, calls.Load(), "durable retry must not touch the upstream again")
	request.Setup.Target.SecureSettings["password"] = "different-secret"
	w = connectionCall(h, "POST", "/api/v1/connections/apply", s.Token, request, identity.RoleAdmin)
	require.Equal(t, 409, w.Code, w.Body.String())
	require.NotContains(t, w.Body.String(), "different-secret")
	w = connectionCall(h, "GET", "/api/v1/connections", s.Token, nil, identity.RoleViewer)
	require.Equal(t, 200, w.Code)
	require.NotContains(t, w.Body.String(), "synthetic-http-password")
	require.Contains(t, w.Body.String(), receipt.DashboardID)
}

type connectionRoundTrip func(*http.Request) (*http.Response, error)

func (f connectionRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestConnectionDiscoveryOnlyKnownLoopbackPortsAndNoWrites(t *testing.T) {
	s, h := connectionTestServer(t)
	var probes atomic.Int32
	s.Collector.Client = &http.Client{Transport: connectionRoundTrip(func(r *http.Request) (*http.Response, error) {
		probes.Add(1)
		require.Equal(t, "127.0.0.1", r.URL.Hostname())
		require.Equal(t, "/metrics", r.URL.Path)
		require.Empty(t, r.URL.RawQuery)
		require.Empty(t, r.Header.Get("Authorization"))
		body := ""
		status := 404
		if r.URL.Port() == "9100" {
			status = 200
			body = "node_cpu_seconds_total{mode=\"idle\"} 233\nnode_memory_MemTotal_bytes 1000\n"
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"text/plain"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	for _, suffix := range []string{"?host=203.0.113.1", "?port=3306"} {
		require.Equal(t, 400, connectionCall(h, "POST", "/api/v1/connections/discover"+suffix, s.Token, nil, identity.RoleAdmin).Code)
	}
	require.Equal(t, 400, connectionCall(h, "POST", "/api/v1/connections/discover", s.Token, map[string]string{"address": "203.0.113.1"}, identity.RoleAdmin).Code)
	require.Zero(t, probes.Load())
	w := connectionCall(h, "POST", "/api/v1/connections/discover", s.Token, nil, identity.RoleAdmin)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"profile":"node"`)
	require.Contains(t, w.Body.String(), `"configured":false`)
	_, err := s.Store.SaveTarget(context.Background(), model.Target{Name: "existing node", URL: "http://localhost:9100/metrics", Kind: "prometheus", IntervalSeconds: 15, Enabled: true})
	require.NoError(t, err)
	w = connectionCall(h, "POST", "/api/v1/connections/discover", s.Token, nil, identity.RoleAdmin)
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"configured":true`)
	metrics, err := s.Store.Metrics(context.Background())
	require.NoError(t, err)
	require.Empty(t, metrics)
	boards, err := s.Store.Dashboards(context.Background())
	require.NoError(t, err)
	require.Len(t, boards, 1)
}

func TestConnectionExistingTargetUsesStoredSecretsAndRejectsChangedReview(t *testing.T) {
	s, h := connectionTestServer(t)
	exporter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, password, ok := r.BasicAuth()
		if !ok || password != "rotated-secret" && password != "original-secret" {
			w.WriteHeader(401)
			return
		}
		fmt.Fprintln(w, "fixture_gauge 20")
	}))
	defer exporter.Close()
	target, err := s.Store.SaveTarget(context.Background(), model.Target{Name: "existing", Kind: "prometheus", URL: exporter.URL, IntervalSeconds: 15, Enabled: true, Labels: map[string]string{"environment": "test"}, SecureSettings: map[string]string{"password": "original-secret"}})
	require.NoError(t, err)
	request := connectionSetupRequest{Setup: model.ConnectionSetupInput{ID: "23300000-0000-4000-8000-000000000006", ExistingTargetID: target.ID, Language: "zh", Alerts: true}}
	w := connectionCall(h, "POST", "/api/v1/connections/preview", s.Token, request, identity.RoleAdmin)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.NotContains(t, w.Body.String(), "original-secret")
	var plan model.ConnectionPlan
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &plan))
	request.PlanHash = plan.PlanHash
	for _, p := range plan.Dashboard.Panels {
		require.Contains(t, p.Expr, `job="existing"`)
		require.NotContains(t, p.Expr, model.ConnectionLabel)
	}
	target.SecureSettings = map[string]string{"password": "rotated-secret"}
	_, err = s.Store.SaveTarget(context.Background(), target)
	require.NoError(t, err)
	w = connectionCall(h, "POST", "/api/v1/connections/apply", s.Token, request, identity.RoleAdmin)
	require.Equal(t, 409, w.Code, w.Body.String())
	w = connectionCall(h, "POST", "/api/v1/connections/preview", s.Token, request, identity.RoleAdmin)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &plan))
	request.PlanHash = plan.PlanHash
	w = connectionCall(h, "POST", "/api/v1/connections/apply", s.Token, request, identity.RoleAdmin)
	require.Equal(t, 200, w.Code, w.Body.String())
	var receipt model.ConnectionReceipt
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &receipt))
	require.Equal(t, target.ID, receipt.TargetID)
	targets, err := s.Store.Targets(context.Background())
	require.NoError(t, err)
	require.Len(t, targets, 1)
	require.Equal(t, map[string]string{"environment": "test"}, targets[0].Labels)
	_, err = s.Store.SaveTarget(context.Background(), model.Target{Name: "duplicate", Kind: "prometheus", URL: exporter.URL, IntervalSeconds: 15, Enabled: true, Labels: map[string]string{"job": "existing", "environment": "test"}})
	require.NoError(t, err)
	request.Setup.ID = "23300000-0000-4000-8000-000000000007"
	w = connectionCall(h, "POST", "/api/v1/connections/preview", s.Token, request, identity.RoleAdmin)
	require.Equal(t, 409, w.Code, w.Body.String())
}
