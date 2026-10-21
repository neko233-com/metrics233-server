package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/stretchr/testify/require"
)

func TestBusinessInstanceCanonicalObservationTemplateAndAllRegionPages(t *testing.T) {
	db, h := setup(t, "")
	labels := map[string]string{"service": "fixture", "role": "logic", "serverId": "0", "instance": "game-0", "environment": "test", "ip": "203.0.113.18"}
	samples := []model.Sample{}
	for i := 0; i < 135; i++ {
		l := map[string]string{}
		for k, v := range labels {
			l[k] = v
		}
		l["country"] = "Fixture"
		l["province"] = "P"
		l["city"] = fmt.Sprintf("City-%03d", i)
		samples = append(samples, model.Sample{Name: "game_players_online_by_region", Labels: l, Value: float64(i)})
	}
	samples = append(samples, model.Sample{Name: "game_players_online", Labels: labels, Value: 9045})
	data, _ := json.Marshal(map[string]any{"samples": samples})
	response := call(h, "POST", "/api/v1/ingest", string(data), "", "")
	require.Equal(t, 200, response.Code, response.Body.String())
	var page store.BusinessInstancePage
	response = call(h, "GET", "/api/v1/business/instances", "", "", "")
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &page))
	require.Len(t, page.Items, 1)
	item := page.Items[0]
	require.Equal(t, "203.0.113.18", item.ReportedIP)
	require.Equal(t, "192.0.2.1", item.ObservedIP) // httptest's socket peer, not the label
	path := "/api/v1/business/instances/" + item.ID
	response = call(h, "PUT", path+"/dashboard", `{"mode":"default","dashboard_id":""}`, "", "")
	require.Equal(t, 400, response.Code)
	response = call(h, "GET", path+"/dashboard", "", "", "")
	require.Equal(t, 200, response.Code, response.Body.String())
	var resolved struct {
		Dashboard model.Dashboard `json:"dashboard"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &resolved))
	require.Len(t, resolved.Dashboard.Panels, 3)
	var scope string
	for _, v := range resolved.Dashboard.Variables {
		if v.Name == "business_scope" {
			scope = v.Current
		}
	}
	require.Contains(t, scope, `serverId="0"`)
	require.Contains(t, scope, `environment="test"`)
	template, err := db.SaveDashboard(context.Background(), model.Dashboard{ID: "business-template", Name: "Live template", Panels: []model.Panel{{ID: "custom", Title: "First", Metric: "game_players_online", Expr: `sum(game_players_online{${business_scope:raw}})`, Aggregation: "last"}}})
	require.NoError(t, err)
	response = call(h, "PUT", path+"/dashboard", `{"version":0,"mode":"inherit","dashboard_id":"business-template"}`, "", "")
	require.Equal(t, 200, response.Code, response.Body.String())
	template.Panels[0].Title = "Template updated"
	_, err = db.SaveDashboardAtRevision(context.Background(), template, template.UpdatedAt)
	require.NoError(t, err)
	response = call(h, "GET", path+"/dashboard", "", "", "")
	require.Equal(t, 200, response.Code)
	require.Contains(t, response.Body.String(), "Template updated")
	response = call(h, "PUT", path+"/dashboard", `{"version":0,"mode":"default","dashboard_id":""}`, "", "")
	require.Equal(t, 409, response.Code)
	seen := map[string]bool{}
	cursor := ""
	var firstCursor string
	for {
		response = call(h, "GET", path+"/data?kind=regions&limit=50&cursor="+cursor, "", "", "")
		require.Equal(t, 200, response.Code, response.Body.String())
		var result struct {
			Items []struct {
				Key    string             `json:"key"`
				Values map[string]float64 `json:"values"`
			} `json:"items"`
			Next  string `json:"next_cursor"`
			Total int    `json:"total"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
		require.Equal(t, 135, result.Total)
		for _, row := range result.Items {
			require.False(t, seen[row.Key])
			seen[row.Key] = true
		}
		cursor = result.Next
		if firstCursor == "" {
			firstCursor = cursor
		}
		if cursor == "" {
			break
		}
	}
	require.Len(t, seen, 135)
	response = call(h, "GET", path+"/data?kind=sql&cursor="+firstCursor, "", "", "")
	require.Equal(t, 400, response.Code)
}

func TestBusinessPopulationSnapshotsExcludeRestartRegionsAndReportPartialStaleAmbiguous(t *testing.T) {
	db, h := setup(t, "")
	ctx := context.Background()
	now := time.Now().UnixMilli()
	base := map[string]string{"service": "snapshot-fixture", "role": "logic", "serverId": "0", "instance": "same-instance", "environment": "production", "channel": "test", "implementation": "go"}
	labels := func(ip, city string) map[string]string {
		out := map[string]string{}
		for k, v := range base {
			out[k] = v
		}
		out["ip"] = ip
		if city != "" {
			out["country"] = "Fixture"
			out["city"] = city
		}
		return out
	}
	require.NoError(t, db.Ingest(ctx, []model.Sample{
		{Name: "game_players_online", Labels: labels("203.0.113.1", ""), Value: 9, Timestamp: now - 120000},
		{Name: "game_player_sessions_started_total", Labels: labels("203.0.113.1", ""), Value: 100, Timestamp: now - 120000},
		{Name: "game_players_online_by_region", Labels: labels("203.0.113.1", "Old"), Value: 9, Timestamp: now - 120000},
		{Name: "game_players_online", Labels: labels("203.0.113.2", ""), Value: 0, Timestamp: now - 2000},
		{Name: "game_player_sessions_started_total", Labels: labels("203.0.113.2", ""), Value: 0, Timestamp: now - 2000},
	}))
	instances, err := db.BusinessInstances(ctx, "", 50)
	require.NoError(t, err)
	require.Len(t, instances.Items, 1)
	id := instances.Items[0].ID
	endpoint := "/api/v1/business/instances/" + id
	type population struct {
		Available  bool     `json:"available"`
		Players    *float64 `json:"players"`
		SnapshotAt int64    `json:"snapshot_at"`
		Stale      bool     `json:"stale"`
		Status     string   `json:"status"`
	}
	type result struct {
		Items []struct {
			Labels map[string]string `json:"labels"`
		} `json:"items"`
		Population population `json:"population"`
		Complete   bool       `json:"regions_complete"`
		Total      float64    `json:"region_total"`
	}
	read := func() result {
		w := call(h, "GET", endpoint+"/data?kind=regions", "", "", "")
		require.Equal(t, 200, w.Code, w.Body.String())
		var out result
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		return out
	}
	zero := read()
	require.Empty(t, zero.Items)
	require.True(t, zero.Complete)
	require.True(t, zero.Population.Available)
	require.Equal(t, 0.0, *zero.Population.Players)
	require.Equal(t, now-2000, zero.Population.SnapshotAt)
	// The built-in online panel also filters old IP label sets, rather than 9+0.
	w := call(h, "GET", endpoint+"/dashboard", "", "", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	var resolved struct {
		Dashboard model.Dashboard `json:"dashboard"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resolved))
	scope := ""
	for _, variable := range resolved.Dashboard.Variables {
		if variable.Name == "business_scope" {
			scope = variable.Current
		}
	}
	expr := strings.ReplaceAll(resolved.Dashboard.Panels[0].Expr, "${business_scope:raw}", scope)
	w = call(h, "GET", "/prometheus/api/v1/query?query="+url.QueryEscape(expr), "", "", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `,"0"]`)
	sessionsExpr := strings.ReplaceAll(resolved.Dashboard.Panels[1].Expr, "${business_scope:raw}", scope)
	w = call(h, "GET", "/prometheus/api/v1/query?query="+url.QueryEscape(sessionsExpr), "", "", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `,"0"]`)
	// A new snapshot can arrive in batches. Earlier regions must not fill its gap.
	require.NoError(t, db.Ingest(ctx, []model.Sample{{Name: "game_players_online", Labels: labels("203.0.113.2", ""), Value: 3, Timestamp: now - 1000}, {Name: "game_players_online_by_region", Labels: labels("203.0.113.2", "New"), Value: 2, Timestamp: now - 1000}}))
	partial := read()
	require.False(t, partial.Complete)
	require.Equal(t, 2.0, partial.Total)
	require.Len(t, partial.Items, 1)
	require.Equal(t, "New", partial.Items[0].Labels["city"])
	require.NoError(t, db.Ingest(ctx, []model.Sample{{Name: "game_players_online_by_region", Labels: labels("203.0.113.2", "Rest"), Value: 1, Timestamp: now - 1000}}))
	complete := read()
	require.True(t, complete.Complete)
	require.Equal(t, 3.0, complete.Total)
	require.Len(t, complete.Items, 2)
	// An equal timestamp is not permission to combine conflicting IP-labelled populations.
	require.NoError(t, db.Ingest(ctx, []model.Sample{{Name: "game_players_online", Labels: labels("203.0.113.3", ""), Value: 4, Timestamp: now - 1000}}))
	ambiguous := read()
	require.False(t, ambiguous.Population.Available)
	require.Equal(t, "ambiguous", ambiguous.Population.Status)
	require.Empty(t, ambiguous.Items)
	w = call(h, "GET", "/prometheus/api/v1/query?query="+url.QueryEscape(expr), "", "", "")
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"result":[]`)
	// Staleness measures sample time, even when the accepted report itself is recent.
	base["instance"] = "stale-instance"
	staleLabels := labels("203.0.113.4", "")
	require.NoError(t, db.Ingest(ctx, []model.Sample{{Name: "game_players_online", Labels: staleLabels, Value: 2, Timestamp: now - 120000}, {Name: "game_players_online_by_region", Labels: labels("203.0.113.4", "Stale"), Value: 2, Timestamp: now - 120000}}))
	id = store.BusinessInstanceID(model.BusinessIdentity{Service: base["service"], Role: base["role"], ServerID: base["serverId"], Instance: base["instance"], Environment: base["environment"], Channel: base["channel"], Implementation: base["implementation"]})
	endpoint = "/api/v1/business/instances/" + id
	stale := read()
	require.True(t, stale.Population.Stale)
	require.True(t, stale.Population.Available)
	base["instance"] = "missing-population"
	require.NoError(t, db.Ingest(ctx, []model.Sample{{Name: "game_players_online_by_region", Labels: labels("203.0.113.5", "Only region"), Value: 7, Timestamp: now - 1000}}))
	id = store.BusinessInstanceID(model.BusinessIdentity{Service: base["service"], Role: base["role"], ServerID: base["serverId"], Instance: base["instance"], Environment: base["environment"], Channel: base["channel"], Implementation: base["implementation"]})
	endpoint = "/api/v1/business/instances/" + id
	missing := read()
	require.False(t, missing.Population.Available)
	require.Nil(t, missing.Population.Players)
	require.Empty(t, missing.Items)
}

func TestBusinessSQLWeightedLatencyAndMissingWindow(t *testing.T) {
	db, h := setup(t, "")
	now := time.Now().UnixMilli()
	samples := []model.Sample{}
	for _, fixture := range []struct {
		table      string
		sum, count float64
	}{{"slow", 1.2, 2}, {"fast", .2, 4}, {"single", 1, 1}} {
		for _, offset := range []int64{120000, 60000, 1000} {
			if fixture.table == "single" && offset != 1000 {
				continue
			}
			factor := float64(121000-offset) / 60000
			for _, metric := range []struct {
				name  string
				value float64
			}{{"game_sql_calls_total", fixture.count * factor}, {"game_sql_failures_total", 0}, {"game_sql_rows_affected_total", fixture.count * 2 * factor}, {"game_sql_duration_seconds_sum", fixture.sum * factor}, {"game_sql_duration_seconds_count", fixture.count * factor}} {
				samples = append(samples, model.Sample{Name: metric.name, Labels: map[string]string{"service": "fixture", "role": "logic", "serverId": "1", "instance": "sql", "table": fixture.table, "operation": "update"}, Timestamp: now - offset, Value: metric.value})
			}
		}
	}
	samples = append(samples, model.Sample{Name: "game_players_online", Labels: map[string]string{"service": "fixture", "role": "logic", "serverId": "1", "instance": "sql"}, Value: 0, Timestamp: now - 1000})
	require.NoError(t, db.Ingest(context.Background(), samples))
	instances, err := db.BusinessInstances(context.Background(), "", 50)
	require.NoError(t, err)
	require.Len(t, instances.Items, 1)
	response := call(h, "GET", "/api/v1/business/instances/"+instances.Items[0].ID+"/data?kind=sql", "", "", "")
	require.Equal(t, 200, response.Code, response.Body.String())
	var result struct {
		Items []struct {
			Labels map[string]string   `json:"labels"`
			Values map[string]*float64 `json:"values"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	require.Len(t, result.Items, 3)
	require.Equal(t, "slow", result.Items[0].Labels["table"])
	require.InDelta(t, 600, *result.Items[0].Values["avg_ms_window"], .001)
	require.Equal(t, "fast", result.Items[1].Labels["table"])
	require.InDelta(t, 50, *result.Items[1].Values["avg_ms_window"], .001)
	require.Equal(t, "single", result.Items[2].Labels["table"])
	require.Nil(t, result.Items[2].Values["avg_ms_window"])
	require.NotNil(t, result.Items[2].Values["calls_total"])
}

func TestBusinessSQLCurrentTotalsFollowPopulationSnapshotWhileWindowRetainsHistory(t *testing.T) {
	db, h := setup(t, "")
	ctx := context.Background()
	now := time.Now().UnixMilli()
	base := map[string]string{"service": "sql-current", "role": "logic", "serverId": "1", "instance": "same-host"}
	labels := func(ip, table string) map[string]string {
		copy := map[string]string{}
		for k, v := range base {
			copy[k] = v
		}
		copy["ip"] = ip
		if table != "" {
			copy["table"] = table
			copy["operation"] = "update"
		}
		return copy
	}
	samples := []model.Sample{{Name: "game_players_online", Labels: labels("203.0.113.2", ""), Value: 0, Timestamp: now - 1000}}
	for _, table := range []string{"shared", "old-only"} {
		for i, offset := range []int64{120000, 60000} {
			factor := float64(i + 1)
			for _, metric := range []struct {
				name  string
				value float64
			}{{"game_sql_calls_total", 10 * factor}, {"game_sql_failures_total", factor}, {"game_sql_rows_affected_total", 20 * factor}, {"game_sql_duration_seconds_sum", 3 * factor}, {"game_sql_duration_seconds_count", 10 * factor}} {
				samples = append(samples, model.Sample{Name: metric.name, Labels: labels("203.0.113.1", table), Value: metric.value, Timestamp: now - offset})
			}
		}
	}
	for _, metric := range []struct {
		name  string
		value float64
	}{{"game_sql_calls_total", 2}, {"game_sql_failures_total", 0}, {"game_sql_rows_affected_total", 4}, {"game_sql_duration_seconds_sum", .2}, {"game_sql_duration_seconds_count", 2}} {
		samples = append(samples, model.Sample{Name: metric.name, Labels: labels("203.0.113.2", "shared"), Value: metric.value, Timestamp: now - 1000})
	}
	require.NoError(t, db.Ingest(ctx, samples))
	id := store.BusinessInstanceID(model.BusinessIdentity{Service: base["service"], Role: base["role"], ServerID: base["serverId"], Instance: base["instance"]})
	type row struct {
		Labels map[string]string   `json:"labels"`
		Values map[string]*float64 `json:"values"`
	}
	read := func(id string) map[string]row {
		w := call(h, "GET", "/api/v1/business/instances/"+id+"/data?kind=sql", "", "", "")
		require.Equal(t, 200, w.Code, w.Body.String())
		var result struct {
			Items []row `json:"items"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		out := map[string]row{}
		for _, item := range result.Items {
			out[item.Labels["table"]] = item
		}
		return out
	}
	rows := read(id)
	require.Len(t, rows, 2)
	shared := rows["shared"]
	require.Equal(t, 2.0, *shared.Values["calls_total"])
	require.Equal(t, 4.0, *shared.Values["rows_total"])
	require.Equal(t, 0.0, *shared.Values["failures_total"])
	require.InDelta(t, 100, *shared.Values["avg_ms_total"], .001)
	require.Greater(t, *shared.Values["calls_window"], 0.0)
	old := rows["old-only"]
	require.Nil(t, old.Values["calls_total"])
	require.Nil(t, old.Values["avg_ms_total"])
	require.Greater(t, *old.Values["calls_window"], 0.0)
	// Missing population does not erase historical window results or invent a current total.
	base["instance"] = "no-population"
	missing := []model.Sample{}
	for i, offset := range []int64{120000, 60000} {
		missing = append(missing, model.Sample{Name: "game_sql_calls_total", Labels: labels("203.0.113.3", "history"), Value: float64(10 + i*10), Timestamp: now - offset})
	}
	require.NoError(t, db.Ingest(ctx, missing))
	id = store.BusinessInstanceID(model.BusinessIdentity{Service: base["service"], Role: base["role"], ServerID: base["serverId"], Instance: base["instance"]})
	rows = read(id)
	require.Greater(t, *rows["history"].Values["calls_window"], 0.0)
	require.Nil(t, rows["history"].Values["calls_total"])
}
