package server_test

import (
	"encoding/json"
	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestSlowSQLAuthenticatedHTTPUsesExactInstanceAndGroupsTemplates(t *testing.T) {
	_, handler := setup(t, "sql-fixture-management")
	samples := []model.Sample{}
	nowMs := time.Now().UnixMilli()
	for _, serverID := range []string{"42", "99"} {
		for _, template := range []string{"select * from player where playerid = ? and level > ?", "select * from player where playerid = ? and level >= ?"} {
			labels := map[string]string{"service": "game", "role": "logic", "serverId": serverID, "instance": "fixture-" + serverID, "environment": "test", "channel": "fixture", "implementation": "go", "table": "Player", "operation": "select", "sql_template": template}
			for _, value := range []struct {
				name          string
				before, after float64
			}{{"game_sql_slow_calls_total", 0, 3}, {"game_sql_slow_duration_seconds_sum", 0, 1.5}, {"game_sql_slow_duration_seconds_max", 0, 0.7}} {
				samples = append(samples, model.Sample{Name: value.name, Labels: labels, Timestamp: nowMs - 120000, Value: value.before}, model.Sample{Name: value.name, Labels: labels, Timestamp: nowMs - 60000, Value: value.after})
			}
		}
	}
	body, _ := json.Marshal(map[string]any{"samples": samples})
	response := call(handler, "POST", "/api/v1/ingest", string(body), "sql-fixture-management", "")
	require.Equal(t, 200, response.Code, response.Body.String())
	response = call(handler, "GET", "/api/v1/business/instances", "", "sql-fixture-management", "")
	require.Equal(t, 200, response.Code)
	var page store.BusinessInstancePage
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &page))
	require.Len(t, page.Items, 2)
	path := "/api/v1/business/instances/" + page.Items[0].ID + "/slow-sql"
	require.Equal(t, 401, call(handler, "GET", path, "", "", "").Code)
	require.Equal(t, 400, call(handler, "GET", path+"?window=24h", "", "sql-fixture-management", "").Code)
	response = call(handler, "GET", path, "", "sql-fixture-management", "")
	require.Equal(t, 200, response.Code, response.Body.String())
	var summary struct {
		Groups []struct {
			Calls    float64 `json:"calls"`
			Variants []any   `json:"variants"`
		} `json:"groups"`
		Tables []any `json:"tables"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &summary))
	require.Len(t, summary.Groups, 1)
	require.Len(t, summary.Groups[0].Variants, 2)
	require.Len(t, summary.Tables, 1)
	// Prometheus increase 带外推，按实例必须只包含两个模板，不能混入另一个实例。
	require.Greater(t, summary.Groups[0].Calls, 5.0)
	require.Less(t, summary.Groups[0].Calls, 20.0)
}

func TestSDKReportingInstanceDoesNotClaimGamePlayerPopulation(t *testing.T) {
	_, handler := setup(t, "")
	response := call(handler, "POST", "/api/v1/ingest", `{"samples":[{"name":"http_connections_active","value":2,"labels":{"service":"sdk","role":"sdk","serverId":"shared","instance":"fixture-sdk","environment":"shared","channel":"shared","implementation":"go"}}]}`, "", "")
	require.Equal(t, 200, response.Code, response.Body.String())
	response = call(handler, "GET", "/api/v1/business/instances", "", "", "")
	var page store.BusinessInstancePage
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &page))
	require.Len(t, page.Items, 1)
	require.Equal(t, "sdk", page.Items[0].Identity.Role)
	response = call(handler, "GET", "/api/v1/business/instances/"+page.Items[0].ID+"/dashboard", "", "", "")
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "sdk_gateway_links_connected")
	require.NotContains(t, response.Body.String(), "game_players_online")
	require.NotContains(t, response.Body.String(), `"population"`)
}
