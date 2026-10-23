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

func TestBusinessResourcesExplicitHostBindingCASAndSafeProjection(t *testing.T) {
	db, h := setup(t, "")
	ctx := context.Background()
	host, err := db.SaveManagedHost(ctx, model.ManagedHost{Name: "Fixture host", Address: "127.0.0.1", Port: 22, User: "private-user", Enabled: true, Target: model.Target{Name: "db", Kind: "mysql", URL: "mysql://127.0.0.1:3306", IntervalSeconds: 30, Enabled: true}, SecureSettings: map[string]string{"ssh_password": "fixture-private-value"}}, "fixture")
	require.NoError(t, err)
	identity := model.BusinessIdentity{Service: "fixture", Role: "logic", ServerID: "1", Instance: "logic", Environment: "test", Implementation: "go"}
	labels := identity.Labels()
	labels["ip"] = "127.0.0.1"
	require.NoError(t, db.Ingest(ctx, []model.Sample{{Name: "process_resident_memory_bytes", Labels: labels, Value: 100}}))
	id := store.BusinessInstanceID(identity)
	path := "/api/v1/business/instances/" + id
	read := func() map[string]json.RawMessage {
		w := call(h, "GET", path+"/dashboard", "", "", "")
		require.Equal(t, 200, w.Code, w.Body.String())
		for _, secret := range []string{"private-user", "fixture-private-value", "ssh_password", "host_key", "mysql://"} {
			require.NotContains(t, w.Body.String(), secret)
		}
		var payload map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
		return payload
	}
	first := read()
	require.Contains(t, string(first["host"]), `"kind":""`) // Matching IP does not auto-bind.
	body := fmt.Sprintf(`{"version":0,"kind":"managed_host","id":%q}`, host.ID)
	w := call(h, "PUT", path+"/host", body, "", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	w = call(h, "PUT", path+"/host", body, "", "")
	require.Equal(t, 409, w.Code)
	bound := read()
	require.Contains(t, string(bound["host"]), "Fixture host")
	var dashboard model.Dashboard
	require.NoError(t, json.Unmarshal(bound["dashboard"], &dashboard))
	ids := map[string]bool{}
	for _, panel := range dashboard.Panels {
		ids[panel.ID] = true
		require.Contains(t, panel.Expr, "${business_scope:raw}")
	}
	for _, id := range []string{"process-cpu", "process-memory", "process-uptime", "process-go-heap", "process-goroutines", "traffic-in", "online"} {
		require.True(t, ids[id], id)
	}
	// Dashboard edits and physical-host association have independent revisions.
	w = call(h, "PUT", path+"/dashboard", `{"version":0,"mode":"default","dashboard_id":""}`, "", "")
	require.Equal(t, 200, w.Code)
	w = call(h, "PUT", path+"/host", `{"version":1,"kind":"","id":""}`, "", "")
	require.Equal(t, 200, w.Code)
	require.Contains(t, string(read()["host"]), `"kind":""`)
}

func TestBusinessProcessQueriesExcludeOtherEnvironmentAndRestartLabels(t *testing.T) {
	db, h := setup(t, "")
	ctx := context.Background()
	now := time.Now()
	base := model.BusinessIdentity{Service: "fixture", Role: "logic", ServerID: "1", Instance: "logic", Environment: "release", Implementation: "go"}
	for _, fixture := range []struct {
		env, ip string
		offset  time.Duration
		memory  float64
	}{{"release", "old", 2 * time.Minute, 999}, {"release", "current", time.Second, 128}, {"test", "same", time.Second, 9000}} {
		i := base
		i.Environment = fixture.env
		l := i.Labels()
		l["ip"] = fixture.ip
		require.NoError(t, db.Ingest(ctx, []model.Sample{{Name: "process_resident_memory_bytes", Labels: l, Value: fixture.memory, Timestamp: now.Add(-fixture.offset).UnixMilli()}, {Name: "process_start_time_seconds", Labels: l, Value: float64(now.Unix() - 300), Timestamp: now.Add(-fixture.offset).UnixMilli()}}))
	}
	w := call(h, "GET", "/api/v1/business/instances/"+store.BusinessInstanceID(base)+"/dashboard", "", "", "")
	require.Equal(t, 200, w.Code)
	var out struct {
		Dashboard model.Dashboard `json:"dashboard"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	scope := ""
	for _, v := range out.Dashboard.Variables {
		if v.Name == "business_scope" {
			scope = v.Current
		}
	}
	for _, p := range out.Dashboard.Panels {
		if !strings.HasPrefix(p.ID, "process-") {
			continue
		}
		expr := strings.ReplaceAll(p.Expr, "${business_scope:raw}", scope)
		q := call(h, "GET", "/prometheus/api/v1/query?query="+url.QueryEscape(expr), "", "", "")
		require.Equal(t, 200, q.Code, q.Body.String())
		if p.ID == "process-memory" {
			require.Contains(t, q.Body.String(), `"128"`)
			require.NotContains(t, q.Body.String(), `"999"`)
		}
		if p.ID == "process-uptime" {
			require.Equal(t, "stat", p.Visualization)
		}
	}
}

func TestBusinessCurrentPanelsRejectOldSourceSamplesAndRetainHistoricalZero(t *testing.T) {
	db, h := setup(t, "")
	ctx, now := context.Background(), time.Now()
	for _, fixture := range []struct {
		id     string
		offset time.Duration
		fresh  bool
	}{{"stale", 2 * time.Minute, false}, {"fresh", time.Second, true}} {
		identity := model.BusinessIdentity{Service: "fixture", Role: "logic", ServerID: fixture.id, Instance: fixture.id}
		at := now.Add(-fixture.offset)
		var samples []model.Sample
		for _, name := range []string{"process_resident_memory_bytes", "game_players_online", "game_player_sessions_started_total"} {
			samples = append(samples, model.Sample{Name: name, Labels: identity.Labels(), Value: 0, Timestamp: at.UnixMilli()})
		}
		require.NoError(t, db.Ingest(ctx, samples))
		w := call(h, "GET", "/api/v1/business/instances/"+store.BusinessInstanceID(identity)+"/dashboard", "", "", "")
		require.Equal(t, 200, w.Code)
		var out struct {
			Dashboard model.Dashboard `json:"dashboard"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		var scope string
		for _, variable := range out.Dashboard.Variables {
			if variable.Name == "business_scope" {
				scope = variable.Current
			}
		}
		for _, panel := range out.Dashboard.Panels {
			if panel.ID != "process-memory" && panel.ID != "online" && panel.ID != "sessions" {
				continue
			}
			expr := strings.ReplaceAll(panel.Expr, "${business_scope:raw}", scope)
			query := "/prometheus/api/v1/query?query=" + url.QueryEscape(expr)
			read := func(path string) int {
				q := call(h, "GET", path, "", "", "")
				require.Equal(t, 200, q.Code, q.Body.String())
				var result struct {
					Data struct {
						Result []struct {
							Value []any `json:"value"`
						} `json:"result"`
					} `json:"data"`
				}
				require.NoError(t, json.Unmarshal(q.Body.Bytes(), &result))
				if len(result.Data.Result) > 0 {
					require.Equal(t, "0", result.Data.Result[0].Value[1], panel.ID)
				}
				return len(result.Data.Result)
			}
			if fixture.fresh {
				require.Equal(t, 1, read(query), panel.ID)
			} else {
				require.Zero(t, read(query), panel.ID+" must not revive a stale value through instant-query evaluation time")
				require.Equal(t, 1, read(query+"&time="+fmt.Sprintf("%.3f", float64(at.Add(time.Second).UnixMilli())/1000)), panel.ID+" must retain valid historical zero")
			}
		}
	}
}

func TestBusinessDataMultipleSortsGlobalBeforeLimitAndCursorScope(t *testing.T) {
	db, h := setup(t, "")
	ctx := context.Background()
	l := map[string]string{"service": "fixture", "role": "logic", "serverId": "1", "instance": "sort"}
	samples := []model.Sample{{Name: "game_players_online", Labels: l, Value: 3}}
	for _, city := range []string{"Alpha", "Gamma", "Beta"} {
		labels := map[string]string{}
		for k, v := range l {
			labels[k] = v
		}
		labels["country"] = "Fixture"
		labels["city"] = city
		samples = append(samples, model.Sample{Name: "game_players_online_by_region", Labels: labels, Value: 1})
	}
	require.NoError(t, db.Ingest(ctx, samples))
	id := store.BusinessInstanceID(model.BusinessIdentity{Service: "fixture", Role: "logic", ServerID: "1", Instance: "sort"})
	path := "/api/v1/business/instances/" + id + "/data?kind=regions&limit=1&sort=value:desc,city:desc"
	w := call(h, "GET", path, "", "", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "Gamma")
	var out struct {
		Next string `json:"next_cursor"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.NotEmpty(t, out.Next)
	w = call(h, "GET", path+"&cursor="+url.QueryEscape(out.Next), "", "", "")
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), "Beta")
	w = call(h, "GET", strings.ReplaceAll(path, "city:desc", "city:asc")+"&cursor="+url.QueryEscape(out.Next), "", "", "")
	require.Equal(t, 400, w.Code)
	for _, invalid := range []string{"value:desc,value:asc", "value:wrong", "private_field:asc", "city:asc,city:desc"} {
		w = call(h, "GET", "/api/v1/business/instances/"+id+"/data?kind=regions&sort="+url.QueryEscape(invalid), "", "", "")
		require.Equal(t, 400, w.Code, invalid)
	}
}

func TestBusinessSQLRankSortsCompleteEligibleSetBeforeTop10(t *testing.T) {
	db, h := setup(t, "")
	ctx := context.Background()
	now := time.Now().UnixMilli()
	identity := model.BusinessIdentity{Service: "fixture", Role: "logic", ServerID: "1", Instance: "rank"}
	samples := []model.Sample{}
	for i := 0; i < 14; i++ {
		labels := identity.Labels()
		labels["table"] = fmt.Sprintf("Entity%02d", i)
		labels["operation"] = "update"
		for n, offset := range []int64{120000, 60000, 1000} {
			for name, value := range map[string]float64{"game_sql_calls_total": float64((n + 1) * (i + 1)), "game_sql_duration_seconds_count": float64((n + 1) * (i + 1)), "game_sql_duration_seconds_sum": float64((n+1)*(i+1)) * .001, "game_sql_failures_total": 0} {
				samples = append(samples, model.Sample{Name: name, Labels: labels, Value: value, Timestamp: now - offset})
			}
		}
	}
	require.NoError(t, db.Ingest(ctx, samples))
	path := "/api/v1/business/instances/" + store.BusinessInstanceID(identity) + "/data?kind=sql&ranked=1&limit=10&sort=calls_window:desc,table:asc"
	w := call(h, "GET", path, "", "", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	var out struct {
		Items []struct {
			Labels map[string]string `json:"labels"`
		} `json:"items"`
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Len(t, out.Items, 10)
	require.Equal(t, 14, out.Total)
	require.Equal(t, "Entity13", out.Items[0].Labels["table"])
	require.Equal(t, "Entity04", out.Items[9].Labels["table"])
}
