package server_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/stretchr/testify/require"
)

func TestBusinessTemplateFilterPagesOnlyExactInheritBindings(t *testing.T) {
	db, handler := setup(t, "")
	ctx := context.Background()
	for _, id := range []string{"gateway-template", "logic-template", "template-copy"} {
		_, err := db.SaveDashboard(ctx, model.Dashboard{ID: id, Name: id, Panels: []model.Panel{}})
		require.NoError(t, err)
	}
	want := map[string]bool{}
	for _, fixture := range []struct{ instance, role, mode, dashboard string }{
		{"gateway-a", "gateway", "inherit", "gateway-template"},
		{"gateway-b", "gateway", "inherit", "gateway-template"},
		{"logic-a", "logic", "inherit", "logic-template"},
		{"independent", "gateway", "independent", "gateway-template"},
		{"copied", "gateway", "inherit", "template-copy"},
		{"default", "gateway", "default", ""},
	} {
		labels := map[string]string{"service": "filter-fixture", "role": fixture.role, "serverId": "0", "instance": fixture.instance, "environment": "test"}
		require.NoError(t, db.RecordBusinessInstances(ctx, []model.Sample{{Name: "gateway_connections_active", Labels: labels}}, ""))
		page, err := db.FindBusinessInstances(ctx, "", fixture.instance, 100)
		require.NoError(t, err)
		require.Len(t, page.Items, 1)
		item := page.Items[0]
		_, err = db.BindBusinessDashboard(ctx, item.ID, model.BusinessBinding{Mode: fixture.mode, DashboardID: fixture.dashboard})
		require.NoError(t, err)
		if fixture.dashboard == "gateway-template" && fixture.mode == "inherit" {
			want[item.ID] = true
		}
	}
	seen := map[string]bool{}
	cursor := ""
	for {
		response := call(handler, "GET", "/api/v1/business/instances?template_id=gateway-template&limit=1&cursor="+cursor, "", "", "")
		require.Equal(t, 200, response.Code, response.Body.String())
		var page store.BusinessInstancePage
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &page))
		require.Len(t, page.Items, 1)
		require.Equal(t, store.BusinessInstanceLimit, page.Capacity)
		require.False(t, page.AtCapacity)
		require.True(t, want[page.Items[0].ID])
		require.False(t, seen[page.Items[0].ID])
		seen[page.Items[0].ID] = true
		cursor = page.Next
		if cursor == "" {
			break
		}
	}
	require.Equal(t, want, seen)
	for query, count := range map[string]int{"gateway-b": 1, "logic-a": 0} {
		response := call(handler, "GET", "/api/v1/business/instances?template_id=gateway-template&q="+url.QueryEscape(query), "", "", "")
		require.Equal(t, 200, response.Code)
		var page store.BusinessInstancePage
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &page))
		require.Len(t, page.Items, count)
	}
	response := call(handler, "GET", "/api/v1/business/instances", "", "", "")
	require.Equal(t, 200, response.Code)
	var unfiltered store.BusinessInstancePage
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &unfiltered))
	require.Len(t, unfiltered.Items, 6)
	for query, status := range map[string]int{
		"template_id=missing": 404,
		"template_id=":        400,
		"template_id=gateway-template&template_id=logic-template": 400,
		"template_id=gateway-template&limit=0":                    400,
		"template_id=gateway-template&cursor=bad":                 400,
		"template_id=%00": 400,
	} {
		response := call(handler, "GET", "/api/v1/business/instances?"+query, "", "", "")
		require.Equal(t, status, response.Code, query+": "+response.Body.String())
	}
}
