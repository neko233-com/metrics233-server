//go:build integration

package integration_test

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These are the existing real database services and authenticated protocol
// fixtures. No probe observations are treated as persisted collection data.
func (e environment) verifyQuickConnection(address string, target model.Target, metric string) {
	e.t.Helper()
	id := uuid.NewSHA1(uuid.NameSpaceURL, []byte("metrics233-docker-quick/"+target.Name)).String()
	input := model.ConnectionSetupInput{ID: id, Target: target, Language: "zh", Alerts: false}
	body := map[string]any{"setup": input}
	preview := e.must(address, "POST", "/api/v1/connections/preview", body)
	var plan model.ConnectionPlan
	require.NoError(e.t, json.Unmarshal(preview, &plan))
	require.NotEmpty(e.t, plan.Dashboard.Panels)
	require.Len(e.t, plan.PlanHash, 64)
	require.Empty(e.t, plan.Rules, "integration must not create outbound notifications")
	for _, secret := range target.SecureSettings {
		require.NotContains(e.t, string(preview), secret)
	}
	body["plan_hash"] = plan.PlanHash
	var first, retry model.ConnectionReceipt
	require.NoError(e.t, json.Unmarshal(e.must(address, "POST", "/api/v1/connections/apply", body), &first))
	require.NoError(e.t, json.Unmarshal(e.must(address, "POST", "/api/v1/connections/apply", body), &retry))
	require.Equal(e.t, first, retry, "same reviewed operation must survive restart/restore without duplication")
	require.Positive(e.t, first.TargetID)
	var board model.Dashboard
	require.NoError(e.t, json.Unmarshal(e.must(address, "GET", "/api/v1/dashboards/"+first.DashboardID, nil), &board))
	require.Equal(e.t, target.Name, board.Name)
	// Scrape through the ordinary collector, then read only the new exact scope.
	e.must(address, "POST", fmt.Sprintf("/api/v1/targets/%d/scrape", first.TargetID), nil)
	labels, _ := json.Marshal(map[string]string{model.ConnectionLabel: id})
	query := url.Values{"metric": {metric}, "labels": {string(labels)}, "range": {"15m"}, "aggregation": {"last"}}
	require.EventuallyWithT(e.t, func(c *assert.CollectT) {
		status, raw, err := e.request(address, "GET", "/api/v1/query?"+query.Encode(), nil)
		require.NoError(c, err)
		require.Equal(c, 200, status)
		var result model.QueryResult
		require.NoError(c, json.Unmarshal(raw, &result))
		require.NotEmpty(c, result.Series)
		for _, series := range result.Series {
			require.Equal(c, id, series.Labels[model.ConnectionLabel])
			require.NotEmpty(c, series.Points)
		}
	}, 10*time.Second, 100*time.Millisecond, "quick connection did not ingest %s in its own scope", target.Kind)
	e.t.Logf("quick-connect %s: actual probe, atomic save, retry, dashboard and scoped ingestion passed", target.Kind)
}
