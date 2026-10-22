package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/metrics233-server/internal/hostagent"
	"github.com/neko233-com/metrics233-server/internal/identity"
	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/stretchr/testify/require"
)

func TestManagedHostsEncryptedCASAndIndependentOutboundCredentials(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "control.db"))
	require.NoError(t, err)
	defer db.DB.Close()
	exporter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprintln(w, "fixture_outbound_value 233") }))
	defer exporter.Close()
	srv := New(db, "synthetic-admin-token", 15)
	srv.ingestToken = "synthetic-ingest-token-0000000000000"
	handler := srv.Handler()
	httpServer := httptest.NewTLSServer(handler)
	defer httpServer.Close()
	srv.Plugins.RootURL = httpServer.URL
	request := func(method, path, token string, body any, role identity.Role) *httptest.ResponseRecorder {
		var data []byte
		if body != nil {
			data, _ = json.Marshal(body)
		}
		r := httptest.NewRequest(method, path, bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if role != "" {
			r = r.WithContext(identity.WithPrincipal(r.Context(), identity.Principal{OrgID: 1, Type: identity.TypeUser, ID: 1, Login: "test-user", Role: role}))
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	h := model.ManagedHost{Name: "Fixture source", Address: "127.0.0.1", Port: 22, User: "metrics", Enabled: true, Target: model.Target{Name: "fixture", Kind: "prometheus", URL: exporter.URL, IntervalSeconds: 60, Labels: map[string]string{"environment": "test", "managed_host": "spoof-attempt"}}, SecureSettings: map[string]string{"ssh_password": "synthetic-ssh-private-credential", "collector_password": "synthetic-db-private-credential"}}
	for _, role := range []identity.Role{identity.RoleViewer, identity.RoleEditor} {
		require.Equal(t, 403, request("POST", "/api/v1/managed-hosts", "", h, role).Code)
		require.Equal(t, 403, request("GET", "/api/v1/managed-hosts", "", nil, role).Code)
	}
	require.Equal(t, 401, request("POST", "/api/v1/managed-hosts", "", h, "").Code)
	w := request("POST", "/api/v1/managed-hosts", srv.Token, h, "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.NotContains(t, w.Body.String(), "private-credential")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &h))
	require.EqualValues(t, 1, h.Version)
	require.True(t, h.SecureFields["ssh_password"])
	var sealed []byte
	require.NoError(t, db.DB.QueryRow(`SELECT secrets FROM managed_hosts WHERE id=?`, h.ID).Scan(&sealed))
	require.NotContains(t, string(sealed), "private-credential")
	_, secrets, err := db.ManagedHost(ctx, h.ID)
	require.NoError(t, err)
	token := secrets["agent_token"]
	require.Len(t, token, 43)
	base := "/agent/v1/hosts/" + h.ID
	for _, bad := range []string{"", srv.Token, "wrong-host-token"} {
		require.Equal(t, 401, request("GET", base+"/configuration", bad, nil, "").Code)
	}
	require.Equal(t, 403, request("GET", base+"/configuration", srv.ingestToken, nil, "").Code)
	require.Equal(t, 401, request("GET", "/api/v1/managed-hosts", token, nil, "").Code)
	require.Equal(t, 403, request("GET", "/api/v1/managed-hosts", srv.ingestToken, nil, "").Code)
	w = request("GET", base+"/configuration", token, nil, "")
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), secrets["collector_password"])
	require.NotContains(t, w.Body.String(), secrets["ssh_password"])
	require.NotContains(t, w.Body.String(), token)
	bootstrap := model.HostBootstrap{Server: httpServer.URL, HostID: h.ID, Token: token}
	agent, err := hostagent.New(bootstrap, "fixture-1")
	require.NoError(t, err)
	agent.Client = httpServer.Client()
	agent.SHA = strings.Repeat("a", 64)
	delay, state, err := agent.Step(ctx)
	require.NoError(t, err)
	require.Equal(t, "collecting", state)
	require.LessOrEqual(t, delay, 60*time.Second)
	result, err := db.Query(ctx, model.Query{Metric: "fixture_outbound_value", Start: time.Now().Add(-time.Minute).UnixMilli(), End: time.Now().UnixMilli() + 1, Step: 1000, Aggregation: "last"})
	require.NoError(t, err)
	require.Len(t, result.Series, 1)
	require.Equal(t, h.ID, result.Series[0].Labels["managed_host"])
	require.Equal(t, 233.0, result.Series[0].Points[0].Value)
	current, _, err := db.ManagedHost(ctx, h.ID)
	require.NoError(t, err)
	require.Equal(t, "collecting", current.AgentState)
	require.Greater(t, current.Samples, 4)
	require.NotNil(t, current.Resources)
	require.Positive(t, current.Resources.MemoryTotal)
	require.Positive(t, current.LastSeen)
	// Saved credentials are preserved when not present in an update. A stale
	// version cannot overwrite a subsequent pause or re-enable collection.
	current.Enabled = false
	w = request("PUT", "/api/v1/managed-hosts/"+h.ID, srv.Token, current, "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, 409, request("PUT", "/api/v1/managed-hosts/"+h.ID, srv.Token, current, "").Code)
	_, preserved, err := db.ManagedHost(ctx, h.ID)
	require.NoError(t, err)
	require.Equal(t, secrets, preserved)
	w = request("GET", base+"/configuration", token, nil, "")
	require.Equal(t, 200, w.Code)
	require.NotContains(t, w.Body.String(), "private-credential")
	_, state, err = agent.Step(ctx)
	require.NoError(t, err)
	require.Equal(t, "paused", state)
	current, _, err = db.ManagedHost(ctx, h.ID)
	require.NoError(t, err)
	require.Equal(t, current.Version, current.AppliedVersion)
	require.Zero(t, current.Samples)
	badReport := model.HostHeartbeat{Version: "fixture", OS: "linux", Arch: "amd64", SHA: strings.Repeat("a", 64), State: "collecting", AppliedVersion: current.Version, Samples: []model.Sample{{Name: "injected", Value: 99}}}
	require.Equal(t, 409, request("POST", base+"/heartbeat", token, badReport, "").Code)
	h2 := h
	h2.ID = ""
	h2.Version = 0
	h2.Name = "Other host"
	w = request("POST", "/api/v1/managed-hosts", srv.Token, h2, "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &h2))
	require.Equal(t, 401, request("GET", "/agent/v1/hosts/"+h2.ID+"/configuration", token, nil, "").Code)
	events := request("GET", "/api/v1/managed-hosts/"+h.ID+"/events", srv.Token, nil, "")
	require.Equal(t, 200, events.Code)
	require.Contains(t, events.Body.String(), "configure")
	require.NotContains(t, events.Body.String(), "private-credential")
	// Explicit plans are guarded by both Admin and revision/pinned-key checks.
	require.Equal(t, 400, request("POST", "/api/v1/managed-hosts/"+h.ID+"/deployment-plan", srv.Token, map[string]int64{"version": current.Version}, "").Code)
	require.Equal(t, 409, request("POST", "/api/v1/managed-hosts/"+h.ID+"/deployment-plan", srv.Token, map[string]int64{"version": 1}, "").Code)
	require.Equal(t, 403, request("POST", "/api/v1/managed-hosts/"+h.ID+"/revoke", "", map[string]int64{"version": current.Version}, identity.RoleEditor).Code)
	require.Equal(t, 200, request("POST", "/api/v1/managed-hosts/"+h.ID+"/revoke", srv.Token, map[string]int64{"version": current.Version}, "").Code)
	require.Equal(t, 401, request("GET", base+"/configuration", token, nil, "").Code)
	revoked, newSecrets, err := db.ManagedHost(ctx, h.ID)
	require.NoError(t, err)
	require.True(t, revoked.Revoked)
	require.False(t, revoked.Enabled)
	require.NotEqual(t, token, newSecrets["agent_token"])
}

func TestHostCredentialRejectsBrowserAndOversizedOrFutureReports(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "control.db"))
	require.NoError(t, err)
	defer db.DB.Close()
	h, err := db.SaveManagedHost(context.Background(), model.ManagedHost{Name: "A", Address: "127.0.0.1", Port: 22, User: "metrics", Enabled: true, Target: model.Target{Name: "mysql", Kind: "mysql", URL: "mysql://localhost:3306", IntervalSeconds: 30}}, "fixture")
	require.NoError(t, err)
	_, secrets, err := db.ManagedHost(context.Background(), h.ID)
	require.NoError(t, err)
	handler := New(db, "admin", 15).Handler()
	path := "/agent/v1/hosts/" + h.ID + "/heartbeat"
	for _, test := range []struct {
		body, origin, cookie string
		status               int
	}{
		{`{}`, "https://example.com", "", 403}, {`{}`, "", "session=anything", 403},
		{`{"version":"v","os":"linux","arch":"amd64","sha":"` + strings.Repeat("a", 64) + `","state":"paused","applied_version":999}`, "", "", 400},
		{`{"unknown":"value"}`, "", "", 400},
	} {
		r := httptest.NewRequest("POST", path, strings.NewReader(test.body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+secrets["agent_token"])
		r.Header.Set("Origin", test.origin)
		r.Header.Set("Cookie", test.cookie)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		require.Equal(t, test.status, w.Code, w.Body.String())
		require.NotContains(t, w.Body.String(), secrets["agent_token"])
	}
}
