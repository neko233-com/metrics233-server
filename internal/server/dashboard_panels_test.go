package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/neko233-com/metrics233-server/internal/grafana"
	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativePanelStateMappingIsExplicitBoundedAndPreservesOpaqueConfig(t *testing.T) {
	db, handler := setup(t, "fixture-state-mapping-admin")
	d, err := db.SaveDashboard(context.Background(), model.Dashboard{ID: "native-state", Name: "State mapping", Panels: []model.Panel{{ID: "state", Title: "State", Aggregation: "last", Expr: "vector(1)", Visualization: "state-timeline", Config: json.RawMessage(`{"id":1,"type":"state-timeline","owner":"keep","opaque":9007199254740993,"fieldConfig":{"defaults":{"unit":"short","mappings":[{"type":"value","options":{"2":{"text":"custom"}}}]},"overrides":[{"keep":true}]},"targets":[{"refId":"A","expr":"vector(1)"}]}`)}}})
	require.NoError(t, err)
	request := func(value, kind string, revision int64, token string) *httptest.ResponseRecorder {
		body := `{"action":"save","sourceId":"state","panel":{"id":"state","title":"State","visualization":` + strconv.Quote(kind) + `,"unit":"count"` + value + `}}`
		r := httptest.NewRequest(http.MethodPatch, "/api/v1/dashboards/"+d.ID+"/panels", bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("If-Match", strconv.FormatInt(revision, 10))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code == http.StatusOK {
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &d))
		}
		return w
	}
	read := func() string {
		current, e := db.Dashboard(context.Background(), d.ID)
		require.NoError(t, e)
		return string(current.Panels[0].Config)
	}
	const token = "fixture-state-mapping-admin"
	require.Equal(t, 401, request(`,"state_mapping":"availability"`, "state-timeline", d.UpdatedAt, "").Code)
	before := read()
	for _, input := range []struct{ value, kind string }{{`,"state_mapping":7`, "state-timeline"}, {`,"state_mapping":true`, "status-history"}, {`,"state_mapping":"other"`, "state-timeline"}, {`,"state_mapping":"availability"`, "stat"}} {
		require.Equal(t, 400, request(input.value, input.kind, d.UpdatedAt, token).Code)
		require.Equal(t, before, read())
	}
	require.Equal(t, 200, request("", "state-timeline", d.UpdatedAt, token).Code)
	require.Contains(t, read(), `"text":"custom"`)
	previous := d.UpdatedAt
	require.Equal(t, 200, request(`,"state_mapping":"availability"`, "status-history", d.UpdatedAt, token).Code)
	configured := read()
	require.Contains(t, configured, `"text":"Available"`)
	require.Contains(t, configured, `"text":"Unavailable"`)
	require.Contains(t, configured, `"owner":"keep"`)
	require.Contains(t, configured, `9007199254740993`)
	require.Contains(t, configured, `"overrides":[{"keep":true}]`)
	require.Contains(t, configured, `"expr":"vector(1)"`)
	require.Equal(t, 409, request(`,"state_mapping":""`, "status-history", previous, token).Code)
	require.Equal(t, configured, read())
	require.Equal(t, 200, request(`,"state_mapping":""`, "status-history", d.UpdatedAt, token).Code)
	require.NotContains(t, read(), `"mappings"`)
}

func TestNativePanelEditAndDuplicatePreserveQueriesAndOpaqueValues(t *testing.T) {
	db, handler := setup(t, "")
	d, err := db.SaveDashboard(context.Background(), model.Dashboard{ID: "native-edit", Name: "Native", Panels: []model.Panel{
		{ID: "first", Title: "First", Aggregation: "last", Visualization: "table", Config: json.RawMessage(`{"id":7,"type":"table","opaque":1e10000,"fieldConfig":{"defaults":{"unit":"ms","opaque":9007199254740993},"overrides":[]},"targets":[{"refId":"A","rawSql":"SELECT 233","datasource":{"uid":"sql","type":"mysql"},"opaque":9007199254740993}]}`)},
		{ID: "other", Title: "Other", Aggregation: "last", Expr: "vector(9)", Config: json.RawMessage(`{"gridPos":{"x":12,"y":0,"w":12,"h":8},"opaque":9007199254740993}`)},
	}})
	require.NoError(t, err)
	d, err = db.Dashboard(context.Background(), d.ID)
	require.NoError(t, err)
	other := d.Panels[1]
	request := func(body string, revision int64) int {
		r := httptest.NewRequest("PATCH", "/api/v1/dashboards/"+d.ID+"/panels", bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		if revision > 0 {
			r.Header.Set("If-Match", strconv.FormatInt(revision, 10))
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code == 200 {
			var saved model.Dashboard
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &saved))
			d = saved
		}
		return w.Code
	}
	baseRevision := d.UpdatedAt
	body := `{"action":"save","sourceId":"first","panel":{"id":"first","title":"Renamed","visualization":"stat","unit":"","expr":"vector(999)"}}`
	require.Equal(t, 200, request(body, d.UpdatedAt))
	assert.Contains(t, string(d.Panels[0].Config), "1e10000")
	assert.Contains(t, string(d.Panels[0].Config), "9007199254740993")
	assert.Contains(t, string(d.Panels[0].Config), `"rawSql":"SELECT 233"`)
	assert.Contains(t, string(d.Panels[0].Config), `"unit":"ms"`)
	assert.Equal(t, other, d.Panels[1])
	assert.Equal(t, "", d.Panels[0].Expr)
	assert.Equal(t, 409, request(body, baseRevision))
	assert.Equal(t, 428, request(body, 0))
	require.Equal(t, 200, request(`{"action":"duplicate","sourceId":"first","panel":{"id":"copied","title":"Copy","visualization":"table","unit":""},"position":{"x":0,"y":8,"w":12,"h":8}}`, d.UpdatedAt))
	require.Len(t, d.Panels, 3)
	assert.Contains(t, string(d.Panels[2].Config), "1e10000")
	assert.Contains(t, string(d.Panels[2].Config), `"id":8`)
	assert.Equal(t, "copied", d.Panels[2].ID)
	assert.Equal(t, 400, request(`{"action":"duplicate","sourceId":"first","panel":{"id":"copied","title":"Duplicate"},"position":{"x":0,"y":16,"w":12,"h":8}}`, d.UpdatedAt))
	require.Equal(t, 200, request(`{"action":"delete","sourceId":"copied"}`, d.UpdatedAt))
	require.Len(t, d.Panels, 2)
	assert.Equal(t, other, d.Panels[1])
	history, err := db.DashboardVersions(context.Background(), d.ID, 1000, 0, "")
	require.NoError(t, err)
	assert.Len(t, history.Versions, 4)
}

func TestNativePanelSaveExportsLegacyPromQLAfterDisplayChanges(t *testing.T) {
	db, handler := setup(t, "")
	d, err := db.SaveDashboard(context.Background(), model.Dashboard{ID: "native-promql-edit", Name: "PromQL", Panels: []model.Panel{{ID: "one", Title: "One", Expr: "vector(233)", Aggregation: "last"}}})
	require.NoError(t, err)
	r := httptest.NewRequest("PATCH", "/api/v1/dashboards/"+d.ID+"/panels", bytes.NewBufferString(`{"action":"save","sourceId":"one","panel":{"id":"one","title":"Gauge","visualization":"gauge","expr":"vector(234)","aggregation":"last","unit":"count"}}`))
	r.Header.Set("If-Match", strconv.FormatInt(d.UpdatedAt, 10))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	require.Equal(t, 200, w.Code, w.Body.String())
	r = httptest.NewRequest("GET", "/api/v1/dashboard-exports/"+d.ID+"?format=grafana", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	require.Equal(t, 200, w.Code, w.Body.String())
	result, err := grafana.Import(w.Body.Bytes())
	require.NoError(t, err)
	require.Len(t, result.Dashboard.Panels, 1)
	assert.Equal(t, "vector(234)", result.Dashboard.Panels[0].Expr)
	assert.Equal(t, "gauge", result.Dashboard.Panels[0].Visualization)
	assert.Equal(t, "Gauge", result.Dashboard.Panels[0].Title)
}

func TestNativePanelDisplayUnitPreservesConfigAndExplicitlyClearsOverride(t *testing.T) {
	db, handler := setup(t, "")
	ctx := context.Background()
	d, err := db.SaveDashboard(ctx, model.Dashboard{ID: "display-unit-edit", Name: "Display unit", Panels: []model.Panel{
		{ID: "rate", Title: "Byte rate", Unit: "bytes", Expr: "rate(gateway_bytes_received_total[5m])", Aggregation: "last", Visualization: "timeseries", Config: json.RawMessage(`{"id":1,"type":"timeseries","description":"fixture description","_metrics233_dashboard_provision_owner":"fixture-owner","opaque":1e10000,"gridPos":{"x":0,"y":0,"w":12,"h":8},"fieldConfig":{"defaults":{"unit":"ms","decimals":3,"thresholds":{"mode":"absolute","steps":[{"color":"red","value":42}]},"color":{"mode":"thresholds"}},"overrides":[{"matcher":{"id":"byName","options":"fixture"},"properties":[{"id":"decimals","value":1}]}]}}`)},
		{ID: "other", Title: "Other", Unit: "count", Expr: "vector(7)", Aggregation: "last"},
	}})
	require.NoError(t, err)
	other := d.Panels[1]
	baseline := d.Panels[0]
	decodeConfig := func(panel model.Panel) (map[string]json.RawMessage, map[string]json.RawMessage, map[string]json.RawMessage) {
		t.Helper()
		var config, fields, defaults map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(panel.Config, &config))
		require.NoError(t, json.Unmarshal(config["fieldConfig"], &fields))
		require.NoError(t, json.Unmarshal(fields["defaults"], &defaults))
		return config, fields, defaults
	}
	baselineConfig, baselineFields, baselineDefaults := decodeConfig(baseline)
	edit := func(unit string, display *string) {
		t.Helper()
		panel := map[string]any{"id": "rate", "title": "Byte rate", "unit": unit, "expr": baseline.Expr, "aggregation": "last", "visualization": "timeseries"}
		if display != nil {
			panel["display_unit"] = *display
		}
		body, err := json.Marshal(map[string]any{"action": "save", "sourceId": "rate", "panel": panel})
		require.NoError(t, err)
		r := httptest.NewRequest(http.MethodPatch, "/api/v1/dashboards/"+d.ID+"/panels", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("If-Match", strconv.FormatInt(d.UpdatedAt, 10))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &d))
		require.Equal(t, other, d.Panels[1])
		config, fields, defaults := decodeConfig(d.Panels[0])
		for _, key := range []string{"description", "_metrics233_dashboard_provision_owner", "opaque", "gridPos"} {
			require.Equal(t, baselineConfig[key], config[key], key)
		}
		require.Equal(t, baselineFields["overrides"], fields["overrides"])
		for _, key := range []string{"thresholds", "decimals", "color"} {
			require.Equal(t, baselineDefaults[key], defaults[key], key)
		}
	}
	assertUnit := func(expected string) {
		t.Helper()
		_, _, defaults := decodeConfig(d.Panels[0])
		require.JSONEq(t, strconv.Quote(expected), string(defaults["unit"]))
	}
	// Legacy/custom display units survive omission, including a base-unit edit.
	edit("bytes", nil)
	assertUnit("ms")
	edit("seconds", nil)
	assertUnit("ms")
	rate, clear := "binBps", ""
	edit("bytes", &rate)
	assertUnit("binBps")
	edit("bytes", nil)
	assertUnit("binBps")
	edit("bytes", &clear)
	assertUnit("bytes")
	edit("seconds", &clear)
	assertUnit("s")
	edit("count", &clear)
	assertUnit("short")
	// Existing basic-unit editing still follows the base when no custom override exists.
	edit("percent", nil)
	assertUnit("percent")
}

func TestNativePanelDisplayUnitRejectsInvalidInputsAndStaleCASWithoutWrites(t *testing.T) {
	db, handler := setup(t, "fixture-management-token")
	ctx := context.Background()
	d, err := db.SaveDashboard(ctx, model.Dashboard{ID: "display-unit-reject", Name: "Display unit", Panels: []model.Panel{{ID: "rate", Title: "Rate", Unit: "bytes", Expr: "vector(1)", Aggregation: "last", Config: json.RawMessage(`{"id":1,"fieldConfig":{"defaults":{"unit":"bytes"},"overrides":[]}}`)}}})
	require.NoError(t, err)
	request := func(unit, display, token string, revision int64) *httptest.ResponseRecorder {
		t.Helper()
		body := `{"action":"save","sourceId":"rate","panel":{"id":"rate","title":"Rate","expr":"vector(1)","aggregation":"last","unit":` + strconv.Quote(unit) + `,"display_unit":` + display + `}}`
		r := httptest.NewRequest(http.MethodPatch, "/api/v1/dashboards/"+d.ID+"/panels", bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if revision != 0 {
			r.Header.Set("If-Match", strconv.FormatInt(revision, 10))
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	for _, input := range []struct{ unit, display string }{
		{"bytes", `"Bps"`}, {"bytes", `"bytes"`}, {"bytes", `"binbps"`},
		{"seconds", `"binBps"`}, {"", `"binBps"`}, {"bytes", `17`}, {"bytes", `true`},
	} {
		w := request(input.unit, input.display, "fixture-management-token", d.UpdatedAt)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		saved, err := db.Dashboard(ctx, d.ID)
		require.NoError(t, err)
		require.Equal(t, d, saved)
	}
	require.Equal(t, http.StatusUnauthorized, request("bytes", `"binBps"`, "", d.UpdatedAt).Code)
	require.Equal(t, http.StatusPreconditionRequired, request("bytes", `"binBps"`, "fixture-management-token", 0).Code)
	oldRevision := d.UpdatedAt
	w := request("bytes", `"binBps"`, "fixture-management-token", oldRevision)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &d))
	w = request("bytes", `""`, "fixture-management-token", oldRevision)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	saved, err := db.Dashboard(ctx, d.ID)
	require.NoError(t, err)
	require.Equal(t, d, saved, "stale clear must not discard the saved override")
}
