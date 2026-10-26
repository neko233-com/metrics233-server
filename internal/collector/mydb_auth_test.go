package collector

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/stretchr/testify/require"
)

func TestMyDBNeverSendsDatabaseCredentialsToMetrics(t *testing.T) {
	var requests atomic.Int32
	token := strings.Repeat("m", 64)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		require.Equal(t, "Bearer "+token, r.Header.Get("Authorization"))
		fmt.Fprint(w, "mydb_up 1\n")
	}))
	defer endpoint.Close()
	collector := New(nil)
	target := model.Target{Kind: "mydb", Name: "mydb", URL: endpoint.URL, IntervalSeconds: 5}
	_, err := collector.collect(context.Background(), target, map[string]string{"password": "database-only"})
	require.ErrorContains(t, err, "Bearer tokens only")
	target.Username = "database-user"
	_, err = collector.collect(context.Background(), target, map[string]string{"bearer_token": token})
	require.ErrorContains(t, err, "Bearer tokens only")
	require.Zero(t, requests.Load(), "no database credential may reach the HTTP endpoint")
	target.Username = ""
	samples, err := collector.collect(context.Background(), target, map[string]string{"bearer_token": token})
	require.NoError(t, err)
	require.Len(t, samples, 1)
	require.Equal(t, int32(1), requests.Load())
}
