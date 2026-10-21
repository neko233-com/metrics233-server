package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/neko233-com/metrics233-server/internal/server"
	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

const ingestOnlyFixtureToken = "synthetic-reporting-only-token-233-0123456789"
const ingestAdminFixtureToken = "synthetic-separate-admin-token-233-0123456789"

func TestIngestOnlyTokenConfigurationFailsClosed(t *testing.T) {
	for _, test := range []struct{ mode, admin, token string }{
		{"unsupported", ingestAdminFixtureToken, ingestOnlyFixtureToken},
		{"token", "", ingestOnlyFixtureToken},
		{"token", "short", ingestOnlyFixtureToken},
		{"token", ingestOnlyFixtureToken, ingestOnlyFixtureToken},
		{"token", ingestAdminFixtureToken, strings.Repeat("x", 31)},
		{"token", ingestAdminFixtureToken, strings.Repeat("x", 8193)},
		{"token", ingestAdminFixtureToken, ingestOnlyFixtureToken + " "},
		{"token", ingestAdminFixtureToken, ingestOnlyFixtureToken + "\n"},
		{"token", ingestAdminFixtureToken, ingestOnlyFixtureToken + "飞书"},
	} {
		err := server.ValidateIngestTokenConfig(test.mode, test.admin, test.token)
		require.Error(t, err)
		require.NotContains(t, err.Error(), ingestOnlyFixtureToken)
	}
	require.NoError(t, server.ValidateIngestTokenConfig("token", ingestAdminFixtureToken, ingestOnlyFixtureToken))
	require.NoError(t, server.ValidateIngestTokenConfig("local", "", ingestOnlyFixtureToken))
	require.NoError(t, server.ValidateIngestTokenConfig("local", ingestAdminFixtureToken, ingestOnlyFixtureToken))
	require.NoError(t, server.ValidateIngestTokenConfig("local", "", ""))
}

func TestIngestOnlyTokenRealHTTPAllFiveProtocolsAndNoManagementAccess(t *testing.T) {
	db, _ := setup(t, ingestAdminFixtureToken)
	application := server.New(db, ingestAdminFixtureToken, 30)
	require.NoError(t, application.ConfigureIngestToken("token", ingestOnlyFixtureToken))
	t.Cleanup(func() { application.Live.Close(); application.Plugins.Close() })
	endpoint := httptest.NewServer(application.Handler())
	defer endpoint.Close()
	client := endpoint.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	now := time.Now().UnixMilli()
	remote, err := proto.Marshal(remoteWriteFixture(now))
	require.NoError(t, err)
	otlp, err := protojson.Marshal(otlpFixture())
	require.NoError(t, err)
	for _, route := range []struct {
		path, kind, encoding string
		body                 []byte
		status               int
	}{
		{"/api/v1/ingest", "application/json", "", []byte(`{"samples":[{"name":"ingest_only_business_total","value":1}]}`), 200},
		{"/api/v1/write", "application/x-protobuf", "snappy", snappy.Encode(nil, remote), 204},
		{"/prometheus/api/v1/write", "application/x-protobuf", "snappy", snappy.Encode(nil, remote), 204},
		{"/v1/metrics", "application/json", "", otlp, 200},
		{"/otlp/v1/metrics", "application/json", "", otlp, 200},
	} {
		request, err := http.NewRequest("POST", endpoint.URL+route.path, bytes.NewReader(route.body))
		require.NoError(t, err)
		request.Header.Set("Authorization", "Bearer "+ingestOnlyFixtureToken)
		request.Header.Set("Content-Type", route.kind)
		request.Header.Set("Content-Encoding", route.encoding)
		response, err := client.Do(request)
		require.NoError(t, err)
		_, _ = io.Copy(io.Discard, response.Body)
		response.Body.Close()
		require.Equal(t, route.status, response.StatusCode, route.path)
	}
	view, err := db.ReportingSources(t.Context())
	require.NoError(t, err)
	require.Len(t, view.Sources, 1)
	require.EqualValues(t, 5, view.Sources[0].AcceptedBatches)
	for _, route := range []struct{ method, path string }{
		{"GET", "/api/v1/query"}, {"GET", "/api/v1/stats"}, {"GET", "/api/v1/reporting-sources"},
		{"PUT", "/api/v1/reporting-sources/policy"}, {"POST", "/api/v1/alerting/contact-points"},
		{"POST", "/api/live/session"}, {"POST", "/api/live/publish/example"}, {"GET", "/metrics"},
		{"GET", "/api/health"}, {"GET", "/"}, {"GET", "/unknown-private-route"}, {"POST", "/unknown-private-route"},
		{"GET", "/api/v1/ingest"}, {"POST", "/api/v1/ingest/"}, {"POST", "/api/v1/ingest/extra"},
	} {
		request, err := http.NewRequest(route.method, endpoint.URL+route.path, strings.NewReader(`{}`))
		require.NoError(t, err)
		request.Header.Set("Authorization", "Bearer "+ingestOnlyFixtureToken)
		response, err := client.Do(request)
		require.NoError(t, err)
		_, _ = io.Copy(io.Discard, response.Body)
		response.Body.Close()
		require.Equal(t, 403, response.StatusCode, route.method+" "+route.path)
	}
	// The administrator retains reads and can block the real socket source.
	request, err := http.NewRequest("GET", endpoint.URL+"/api/v1/reporting-sources", nil)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+ingestAdminFixtureToken)
	response, err := client.Do(request)
	require.NoError(t, err)
	response.Body.Close()
	require.Equal(t, 200, response.StatusCode)
	require.NoError(t, db.UpdateReportingPolicy(t.Context(), store.ReportingPolicyUpdate{Version: view.Version, Address: view.Sources[0].Address, Action: "block"}))
	request, err = http.NewRequest("POST", endpoint.URL+"/api/v1/ingest", strings.NewReader(`{"samples":[{"name":"blocked_after_ingest_token","value":9}]}`))
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+ingestOnlyFixtureToken)
	response, err = client.Do(request)
	require.NoError(t, err)
	response.Body.Close()
	require.Equal(t, 403, response.StatusCode)
	view, err = db.ReportingSources(context.Background())
	require.NoError(t, err)
	require.Equal(t, "block", view.Sources[0].Disposition)
	require.EqualValues(t, 5, view.Sources[0].AcceptedBatches)
}

func TestIngestOnlyTokenRejectsAlternateAndDuplicateCredentials(t *testing.T) {
	db, _ := setup(t, ingestAdminFixtureToken)
	application := server.New(db, ingestAdminFixtureToken, 30)
	require.NoError(t, application.ConfigureIngestToken("token", ingestOnlyFixtureToken))
	t.Cleanup(func() { application.Live.Close(); application.Plugins.Close() })
	endpoint := httptest.NewServer(application.Handler())
	defer endpoint.Close()
	for _, headers := range [][]string{nil, {"Bearer wrong"}, {ingestOnlyFixtureToken}, {"bearer " + ingestOnlyFixtureToken}, {"Bearer " + ingestOnlyFixtureToken, "Bearer " + ingestOnlyFixtureToken}, {"Bearer " + ingestAdminFixtureToken, "Bearer " + ingestOnlyFixtureToken}, {"Bearer " + ingestOnlyFixtureToken, "Bearer " + ingestAdminFixtureToken}} {
		request, err := http.NewRequest("POST", endpoint.URL+"/api/v1/ingest?token="+ingestOnlyFixtureToken+"&api_key="+ingestOnlyFixtureToken, strings.NewReader(`{"samples":[{"name":"must_not_write","value":1}]}`))
		require.NoError(t, err)
		for _, header := range headers {
			request.Header.Add("Authorization", header)
		}
		request.AddCookie(&http.Cookie{Name: "token", Value: ingestOnlyFixtureToken})
		response, err := endpoint.Client().Do(request)
		require.NoError(t, err)
		_, _ = io.Copy(io.Discard, response.Body)
		response.Body.Close()
		require.Equal(t, 401, response.StatusCode)
	}
	metrics, err := db.Metrics(t.Context())
	require.NoError(t, err)
	require.Empty(t, metrics)
	view, err := db.ReportingSources(t.Context())
	require.NoError(t, err)
	require.Empty(t, view.Sources)
	// A management credential still returns full metadata, never either token.
	request, err := http.NewRequest("GET", endpoint.URL+"/api/v1/reporting-sources", nil)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+ingestAdminFixtureToken)
	response, err := endpoint.Client().Do(request)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&result))
	response.Body.Close()
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), ingestAdminFixtureToken)
	require.NotContains(t, string(encoded), ingestOnlyFixtureToken)
}
