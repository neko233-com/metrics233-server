package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

// Exercise real transports and valid protocol bodies: a parser rejection must
// never be mistaken for an authentication rejection or a successful no-op.
func TestFixedTokenGuardsEveryMetricIngestTransport(t *testing.T) {
	const fixed = "synthetic-fixed-token-233"
	now := time.Now().UnixMilli()
	remote, err := proto.Marshal(remoteWriteFixture(now))
	require.NoError(t, err)
	otlp, err := protojson.Marshal(otlpFixture())
	require.NoError(t, err)
	native, err := json.Marshal(map[string]any{"samples": []map[string]any{{"name": "business_auth_total", "value": 1, "timestamp": now}}})
	require.NoError(t, err)
	for _, route := range []struct {
		path, contentType, encoding string
		body                        []byte
		accepted                    int
	}{
		{"/api/v1/ingest", "application/json", "", native, 200},
		{"/api/v1/write", "application/x-protobuf", "snappy", snappy.Encode(nil, remote), 204},
		{"/prometheus/api/v1/write", "application/x-protobuf", "snappy", snappy.Encode(nil, remote), 204},
		{"/v1/metrics", "application/json", "", otlp, 200},
		{"/otlp/v1/metrics", "application/json", "", otlp, 200},
	} {
		t.Run(route.path, func(t *testing.T) {
			db, handler := setup(t, fixed)
			server := httptest.NewServer(handler)
			defer server.Close()
			for _, header := range []string{"", "Bearer incorrect-fixture-token"} {
				request, err := http.NewRequest("POST", server.URL+route.path+"?token="+fixed+"&api_key="+fixed, bytes.NewReader(route.body))
				require.NoError(t, err)
				request.Header.Set("Content-Type", route.contentType)
				request.Header.Set("Content-Encoding", route.encoding)
				request.Header.Set("Authorization", header)
				request.AddCookie(&http.Cookie{Name: "token", Value: fixed})
				response, err := server.Client().Do(request)
				require.NoError(t, err)
				_, _ = io.Copy(io.Discard, response.Body)
				response.Body.Close()
				require.Equal(t, http.StatusUnauthorized, response.StatusCode)
				metrics, err := db.Metrics(context.Background())
				require.NoError(t, err)
				require.Empty(t, metrics, "an unauthorized valid payload reached metric storage")
			}
			request, err := http.NewRequest("POST", server.URL+route.path, bytes.NewReader(route.body))
			require.NoError(t, err)
			request.Header.Set("Content-Type", route.contentType)
			request.Header.Set("Content-Encoding", route.encoding)
			request.Header.Set("Authorization", "Bearer "+fixed)
			response, err := server.Client().Do(request)
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, response.Body)
			response.Body.Close()
			require.Equal(t, route.accepted, response.StatusCode)
			metrics, err := db.Metrics(context.Background())
			require.NoError(t, err)
			require.Len(t, metrics, 1, "the authenticated valid payload must reach storage")
		})
	}
}
