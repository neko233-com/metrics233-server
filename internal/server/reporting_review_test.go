package server_test

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/neko233-com/metrics233-server/internal/identity"
	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/server"
	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

type reportingReviewBody struct {
	io.Reader
	reads int
}

func (b *reportingReviewBody) Read(p []byte) (int, error) { b.reads++; return b.Reader.Read(p) }

type reportingReviewBackend struct {
	store.MetricBackend
	calls atomic.Int32
}

func (b *reportingReviewBackend) Ingest(context.Context, []model.Sample) error {
	b.calls.Add(1)
	return nil
}

func reportingReviewRoutes(t *testing.T) []struct {
	path, kind, encoding string
	body                 []byte
	status               int
} {
	t.Helper()
	remote, err := proto.Marshal(remoteWriteFixture(time.Now().UnixMilli()))
	require.NoError(t, err)
	otlp, err := protojson.Marshal(otlpFixture())
	require.NoError(t, err)
	return []struct {
		path, kind, encoding string
		body                 []byte
		status               int
	}{
		{"/api/v1/ingest", "application/json", "", []byte(`{"samples":[{"name":"reporting_review_total","value":1}]}`), 200},
		{"/api/v1/write", "application/x-protobuf", "snappy", snappy.Encode(nil, remote), 204},
		{"/prometheus/api/v1/write", "application/x-protobuf", "snappy", snappy.Encode(nil, remote), 204},
		{"/v1/metrics", "application/json", "", otlp, 200},
		{"/otlp/v1/metrics", "application/json", "", otlp, 200},
	}
}

func TestReportingReviewPolicyFailureIsClosedBeforeBodyOrMetricBackend(t *testing.T) {
	for _, route := range reportingReviewRoutes(t) {
		t.Run(route.path, func(t *testing.T) {
			db, handler := setup(t, ingestAdminFixtureToken)
			backend := &reportingReviewBackend{}
			db.Backend = backend
			_, err := db.DB.Exec(`DROP TABLE metric_reporting_settings`)
			require.NoError(t, err)
			for _, token := range []string{"incorrect-token", ingestAdminFixtureToken} {
				body := &reportingReviewBody{Reader: bytes.NewReader(route.body)}
				request := httptest.NewRequest("POST", route.path, body)
				request.RemoteAddr = "192.0.2.17:1234"
				request.Header.Set("Authorization", "Bearer "+token)
				request.Header.Set("Content-Type", route.kind)
				request.Header.Set("Content-Encoding", route.encoding)
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				status := 503
				if token != ingestAdminFixtureToken {
					status = 401
				}
				require.Equal(t, status, response.Code, response.Body.String())
				require.Zero(t, body.reads)
				require.Zero(t, backend.calls.Load())
			}
		})
	}
}

func TestReportingReviewMetadataFailureNeverTurnsAcceptedMetricsIntoRetry(t *testing.T) {
	for _, route := range reportingReviewRoutes(t) {
		t.Run(route.path, func(t *testing.T) {
			db, handler := setup(t, ingestAdminFixtureToken)
			_, err := db.DB.Exec(`CREATE TRIGGER synthetic_reporting_failure BEFORE INSERT ON metric_reporting_observations BEGIN SELECT RAISE(ABORT,'synthetic auxiliary failure'); END`)
			require.NoError(t, err)
			request := httptest.NewRequest("POST", route.path, bytes.NewReader(route.body))
			request.RemoteAddr = "192.0.2.17:1234"
			request.Header.Set("Authorization", "Bearer "+ingestAdminFixtureToken)
			request.Header.Set("Content-Type", route.kind)
			request.Header.Set("Content-Encoding", route.encoding)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			require.Equal(t, route.status, response.Code, response.Body.String())
			metrics, err := db.Metrics(t.Context())
			require.NoError(t, err)
			require.Len(t, metrics, 1)
			view, err := db.ReportingSources(t.Context())
			require.NoError(t, err)
			require.Empty(t, view.Sources)
		})
	}
}

func TestReportingReviewPartialRemoteWriteCountsOnlyCommittedBatches(t *testing.T) {
	db, handler := setup(t, ingestAdminFixtureToken)
	backend := &interruptedIngestBackend{failAt: 2}
	db.Backend = backend
	fixture := remoteWriteFixture(time.Now().UnixMilli())
	point := fixture.Timeseries[0].Samples[0]
	fixture.Timeseries[0].Samples = make([]prompb.Sample, 10001)
	for i := range fixture.Timeseries[0].Samples {
		fixture.Timeseries[0].Samples[i] = point
	}
	body, err := proto.Marshal(fixture)
	require.NoError(t, err)
	request := httptest.NewRequest("POST", "/api/v1/write", bytes.NewReader(body))
	request.RemoteAddr = "192.0.2.17:1234"
	request.Header.Set("Authorization", "Bearer "+ingestAdminFixtureToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, 503, response.Code)
	require.Equal(t, "10000", response.Header().Get("X-Accepted-Samples"))
	require.Equal(t, 10000, backend.accepted)
	view, err := db.ReportingSources(t.Context())
	require.NoError(t, err)
	require.Len(t, view.Sources, 1)
	require.EqualValues(t, 10000, view.Sources[0].AcceptedSamples)
	require.EqualValues(t, 1, view.Sources[0].AcceptedBatches)
}

func TestReportingReviewMappedIPsAndTrustCannotBypassBlock(t *testing.T) {
	db, _ := setup(t, ingestAdminFixtureToken)
	app := server.New(db, ingestAdminFixtureToken, 30)
	require.NoError(t, app.ConfigureReportingProxies([]string{"127.0.0.1/32"}))
	handler := app.Handler()
	t.Cleanup(func() { app.Live.Close(); app.Plugins.Close() })
	view, err := db.ReportingSources(t.Context())
	require.NoError(t, err)
	require.NoError(t, db.UpdateReportingPolicy(t.Context(), store.ReportingPolicyUpdate{Version: view.Version, Address: "192.0.2.17", Action: "block"}))
	for _, test := range []struct {
		peer      string
		forwarded []string
	}{
		{"[::ffff:192.0.2.17]:1234", []string{"198.51.100.10"}},
		{"::ffff:192.0.2.17", []string{"198.51.100.10"}},
		{"[::ffff:127.0.0.1]:1234", []string{"::ffff:192.0.2.17"}},
		{"127.0.0.1:1234", []string{"198.51.100.10", "::ffff:192.0.2.17"}},
		{"127.0.0.1:1234", []string{"198.51.100.10, ::ffff:192.0.2.17"}},
	} {
		request := httptest.NewRequest("POST", "/api/v1/ingest", bytes.NewBufferString(`{"samples":[{"name":"must_not_write","value":1}]}`))
		request.RemoteAddr = test.peer
		request.Header.Set("Authorization", "Bearer "+ingestAdminFixtureToken)
		for _, header := range test.forwarded {
			request.Header.Add("X-Forwarded-For", header)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		require.Equal(t, 403, response.Code, response.Body.String())
	}
	metrics, err := db.Metrics(t.Context())
	require.NoError(t, err)
	require.Empty(t, metrics)
}

func TestReportingReviewPhysicalOrganizationPoliciesAndPrincipalBoundary(t *testing.T) {
	primary, handler := setup(t, ingestAdminFixtureToken)
	secondary, err := store.OpenForOrganization(filepath.Join(t.TempDir(), "org2", "control.db"), 2)
	require.NoError(t, err)
	defer secondary.DB.Close()
	view, err := primary.ReportingSources(t.Context())
	require.NoError(t, err)
	require.NoError(t, primary.UpdateReportingPolicy(t.Context(), store.ReportingPolicyUpdate{Version: view.Version, Address: "192.0.2.17", Action: "block"}))
	allowed, err := secondary.ReportingSourceAllowed(t.Context(), "192.0.2.17")
	require.NoError(t, err)
	require.True(t, allowed)
	for _, wrongPrincipal := range []bool{false, true} {
		request := httptest.NewRequest("POST", "/api/v1/ingest", bytes.NewBufferString(`{"samples":[{"name":"must_not_write","value":1}]}`))
		request.RemoteAddr = "192.0.2.17:1234"
		request.Header.Set("Authorization", "Bearer "+ingestAdminFixtureToken)
		request.Header.Set("X-Grafana-Org-Id", "2")
		if wrongPrincipal {
			request = request.WithContext(identity.WithPrincipal(request.Context(), identity.Principal{OrgID: 2, ID: 2, Type: identity.TypeUser, Login: "other-org", Role: identity.RoleAdmin}))
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		require.Equal(t, 403, response.Code, response.Body.String())
	}
	other, err := secondary.ReportingSources(t.Context())
	require.NoError(t, err)
	require.Empty(t, other.Sources)
	metrics, err := primary.Metrics(t.Context())
	require.NoError(t, err)
	require.Empty(t, metrics)
	// Policy read errors are returned to callers rather than interpreted as allow.
	_, err = secondary.DB.Exec(`DELETE FROM metric_reporting_settings`)
	require.NoError(t, err)
	allowed, err = secondary.ReportingSourceAllowed(t.Context(), "192.0.2.17")
	require.Error(t, err)
	require.False(t, allowed)
}
