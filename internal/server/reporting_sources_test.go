package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/neko233-com/metrics233-server/internal/server"
	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestReportingPolicyGuardsAllProtocolsAndPersists(t *testing.T) {
	now := time.Now().UnixMilli()
	remote, err := proto.Marshal(remoteWriteFixture(now))
	require.NoError(t, err)
	otlp, err := protojson.Marshal(otlpFixture())
	require.NoError(t, err)
	for _, route := range []struct {
		path, kind, encoding string
		body                 []byte
	}{
		{"/api/v1/ingest", "application/json", "", []byte(`{"samples":[{"name":"source_test","labels":{"service":"shop"},"value":1}]}`)},
		{"/api/v1/write", "application/x-protobuf", "snappy", snappy.Encode(nil, remote)},
		{"/prometheus/api/v1/write", "application/x-protobuf", "snappy", snappy.Encode(nil, remote)},
		{"/v1/metrics", "application/json", "", otlp},
		{"/otlp/v1/metrics", "application/json", "", otlp},
	} {
		t.Run(route.path, func(t *testing.T) {
			db, h := setup(t, "fixture-fixed-token")
			request := func(token string) *httptest.ResponseRecorder {
				r := httptest.NewRequest("POST", route.path, bytes.NewReader(route.body))
				r.RemoteAddr = "192.0.2.17:1234"
				r.Header.Set("Content-Type", route.kind)
				r.Header.Set("Content-Encoding", route.encoding)
				r.Header.Set("Authorization", "Bearer "+token)
				// Neither a spoofed forwarding address nor service labels override policy.
				r.Header.Set("X-Forwarded-For", "198.51.100.27")
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				return w
			}
			require.Equal(t, 401, request("wrong").Code)
			view, err := db.ReportingSources(context.Background())
			require.NoError(t, err)
			require.Empty(t, view.Sources)
			require.NoError(t, db.UpdateReportingPolicy(context.Background(), store.ReportingPolicyUpdate{Version: view.Version, Address: "::ffff:192.0.2.17", Action: "block"}))
			require.Equal(t, 403, request("fixture-fixed-token").Code)
			metrics, err := db.Metrics(context.Background())
			require.NoError(t, err)
			require.Empty(t, metrics)
			view, err = db.ReportingSources(context.Background())
			require.NoError(t, err)
			require.Len(t, view.Sources, 1)
			require.Zero(t, view.Sources[0].LastSeen)
			require.NoError(t, db.UpdateReportingPolicy(context.Background(), store.ReportingPolicyUpdate{Version: view.Version, Address: "192.0.2.17", Action: "allow", AllowlistOnly: true}))
			response := request("fixture-fixed-token")
			require.Contains(t, []int{200, 204}, response.Code, response.Body.String())
			view, err = db.ReportingSources(context.Background())
			require.NoError(t, err)
			require.Len(t, view.Sources, 1)
			require.Equal(t, "192.0.2.17", view.Sources[0].Address)
			require.Positive(t, view.Sources[0].AcceptedSamples)
			require.EqualValues(t, 1, view.Sources[0].AcceptedBatches)
			require.Positive(t, view.Sources[0].LastSeen)
			// Removing an allow entry in allowlist-only mode must deny future writes.
			require.NoError(t, db.UpdateReportingPolicy(context.Background(), store.ReportingPolicyUpdate{Version: view.Version, Address: "192.0.2.17", Action: "reset", AllowlistOnly: true}))
			require.Equal(t, 403, request("fixture-fixed-token").Code)
			require.ErrorIs(t, db.UpdateReportingPolicy(context.Background(), store.ReportingPolicyUpdate{Version: view.Version, AllowlistOnly: false, Address: "192.0.2.17", Action: "allow"}), store.ErrReportingPolicyConflict)
		})
	}
}

func TestReportingSourcesTrustAndAdminAPI(t *testing.T) {
	db, _ := setup(t, "fixture-fixed-token")
	srv := server.New(db, "fixture-fixed-token", 7)
	require.NoError(t, srv.ConfigureReportingProxies([]string{"127.0.0.1/32", "10.0.0.0/8"}))
	h := srv.Handler()
	push := func(peer, forwarded string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/v1/ingest", bytes.NewBufferString(`{"samples":[{"name":"source_trust","value":1}]}`))
		r.RemoteAddr = peer
		r.Header.Set("Authorization", "Bearer fixture-fixed-token")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Add("X-Forwarded-For", "203.0.113.99")
		r.Header.Add("X-Forwarded-For", forwarded)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	require.Equal(t, 200, push("127.0.0.1:1234", "198.51.100.19, 10.0.0.2").Code)
	require.Equal(t, 200, push("192.0.2.22:1234", "198.51.100.19").Code)
	view, err := db.ReportingSources(context.Background())
	require.NoError(t, err)
	addresses := []string{}
	for _, source := range view.Sources {
		addresses = append(addresses, source.Address)
	}
	require.ElementsMatch(t, []string{"198.51.100.19", "192.0.2.22"}, addresses)
	update := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("PUT", "/api/v1/reporting-sources/policy", bytes.NewBufferString(body))
		r.Header.Set("Authorization", "Bearer fixture-fixed-token")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	encoded, _ := json.Marshal(store.ReportingPolicyUpdate{Version: view.Version, Address: "198.51.100.19", Action: "block"})
	require.Equal(t, 200, update(string(encoded)).Code)
	require.Equal(t, 409, update(string(encoded)).Code)
	require.Equal(t, 403, push("127.0.0.1:1234", "198.51.100.19, 10.0.0.2").Code)
	require.Equal(t, 400, update(`{"version":2,"address":"192.0.2.22","action":"block"}`).Code)
	// Reopening the physical catalog retains both decisions and observations.
	reopened, err := store.Open(db.Path)
	require.NoError(t, err)
	defer reopened.DB.Close()
	allowed, err := reopened.ReportingSourceAllowed(context.Background(), "198.51.100.19")
	require.NoError(t, err)
	require.False(t, allowed)
	persisted, err := reopened.ReportingSources(context.Background())
	require.NoError(t, err)
	require.Len(t, persisted.Sources, 2)
}
