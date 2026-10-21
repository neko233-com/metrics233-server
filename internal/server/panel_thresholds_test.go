package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/stretchr/testify/require"
)

func TestNativePanelThresholdEditPreservesQueriesValidatesAndClears(t *testing.T) {
	db, h := setup(t, "")
	d, err := db.SaveDashboard(context.Background(), model.Dashboard{ID: "threshold-fixture", Name: "Risk fixture", Panels: []model.Panel{{ID: "cpu", Title: "CPU", Aggregation: "last", Visualization: "timeseries", Unit: "percent", Config: json.RawMessage(`{"targets":[{"expr":"fixture_cpu"}],"fieldConfig":{"defaults":{"opaque":9007199254740993},"overrides":[]}}`)}}})
	require.NoError(t, err)
	call := func(thresholds string, version int64) *httptest.ResponseRecorder {
		r := httptest.NewRequest("PATCH", "/api/v1/dashboards/threshold-fixture/panels", bytes.NewBufferString(`{"action":"save","sourceId":"cpu","panel":{"id":"cpu","title":"CPU","visualization":"timeseries","unit":"percent","thresholds":`+thresholds+`}}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("If-Match", strconv.FormatInt(version, 10))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, value := range []string{`{}`, `{"warning":85}`, `{"warning":95,"critical":85}`, `{"warning":85,"critical":85}`, `{"warning":85,"critical":1e10000}`, `{"warning":85,"critical":95,"extra":1}`} {
		require.Equal(t, 400, call(value, d.UpdatedAt).Code)
	}
	w := call(`{"warning":85,"critical":95}`, d.UpdatedAt)
	require.Equal(t, 200, w.Code, w.Body.String())
	var updated model.Dashboard
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
	require.Contains(t, string(updated.Panels[0].Config), `"expr":"fixture_cpu"`)
	require.Contains(t, string(updated.Panels[0].Config), `9007199254740993`)
	require.Contains(t, string(updated.Panels[0].Config), `"color":"yellow","value":85`)
	require.Equal(t, 409, call(`null`, d.UpdatedAt).Code)
	w = call(`null`, updated.UpdatedAt)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
	require.NotContains(t, string(updated.Panels[0].Config), `"steps"`)
	require.Contains(t, string(updated.Panels[0].Config), `fixture_cpu`)
}
