package instance

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/stretchr/testify/require"
)

func TestResourceReadHTTPBusinessTemplateFilterChecksACLBeforeBindings(t *testing.T) {
	f := newResourceReadFixture(t)
	ctx := context.Background()
	for _, uid := range []string{"visible-template", "hidden-template"} {
		_, err := f.primary.Store.SaveDashboard(ctx, model.Dashboard{ID: uid, Name: uid, Panels: []model.Panel{}})
		require.NoError(t, err)
		labels := map[string]string{"service": uid, "role": "gateway", "serverId": "0", "instance": uid}
		require.NoError(t, f.primary.Store.RecordBusinessInstances(ctx, []model.Sample{{Name: "gateway_connections_active", Labels: labels}}, ""))
		page, err := f.primary.Store.FindBusinessInstances(ctx, "", uid, 1)
		require.NoError(t, err)
		require.Len(t, page.Items, 1)
		_, err = f.primary.Store.BindBusinessDashboard(ctx, page.Items[0].ID, model.BusinessBinding{Mode: "inherit", DashboardID: uid})
		require.NoError(t, err)
	}
	f.start(t)
	f.grant(t, model.PermissionDashboard, "visible-template", model.PermissionView)
	status, body := f.get(t, "/api/v1/business/instances?template_id=visible-template")
	require.Equal(t, http.StatusOK, status, body)
	var page store.BusinessInstancePage
	require.NoError(t, json.Unmarshal([]byte(body), &page))
	require.Len(t, page.Items, 1)
	require.Equal(t, "visible-template", page.Items[0].Binding.DashboardID)
	for path, expected := range map[string]int{
		"/api/v1/business/instances?template_id=hidden-template": http.StatusForbidden,
		"/api/v1/business/instances?template_id=missing":         http.StatusNotFound,
		"/api/v1/business/instances":                             http.StatusForbidden,
	} {
		status, body = f.get(t, path)
		require.Equal(t, expected, status, body)
		require.NotContains(t, body, "hidden-template")
	}
	// A former grant cannot keep exposing instance metadata after revocation.
	ref := model.PermissionResource{OrgID: 1, Kind: model.PermissionDashboard, UID: "visible-template"}
	doc, err := f.primary.Store.ResourcePermissions(f.internal, ref)
	require.NoError(t, err)
	_, err = f.primary.Store.ReplaceResourcePermissions(f.internal, ref, doc.Version, nil, f.resolver)
	require.NoError(t, err)
	status, body = f.get(t, "/api/v1/business/instances?template_id=visible-template")
	require.Equal(t, http.StatusForbidden, status, body)
}

func TestResourceReadHTTPBusinessTemplateFilterRequiresLibraryOwnerPermission(t *testing.T) {
	f := newResourceReadFixture(t)
	ctx := context.Background()
	_, err := f.primary.Store.CreateFolder(ctx, model.Folder{UID: "template-library-owner", Title: "Private owner"})
	require.NoError(t, err)
	_, err = f.primary.Store.SaveLibraryElement(ctx, model.LibraryElement{UID: "template-library", Name: "Private library", Kind: 1, FolderUID: "template-library-owner", Model: json.RawMessage(`{"type":"text","options":{"content":"private fixture"}}`)}, true)
	require.NoError(t, err)
	_, err = f.primary.Store.SaveDashboard(ctx, model.Dashboard{ID: "linked-template", Name: "Linked template", Panels: []model.Panel{{ID: "linked", Title: "Reference", Aggregation: "last", Config: json.RawMessage(`{"id":1,"type":"text","libraryPanel":{"uid":"template-library"}}`)}}})
	require.NoError(t, err)
	labels := map[string]string{"service": "library-fixture", "role": "gateway", "serverId": "0", "instance": "library-instance"}
	require.NoError(t, f.primary.Store.RecordBusinessInstances(ctx, []model.Sample{{Name: "gateway_connections_active", Labels: labels}}, ""))
	page, err := f.primary.Store.BusinessInstances(ctx, "", 1)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	_, err = f.primary.Store.BindBusinessDashboard(ctx, page.Items[0].ID, model.BusinessBinding{Mode: "inherit", DashboardID: "linked-template"})
	require.NoError(t, err)
	f.start(t)
	f.grant(t, model.PermissionDashboard, "linked-template", model.PermissionView)
	status, body := f.get(t, "/api/v1/business/instances?template_id=linked-template")
	require.Equal(t, http.StatusForbidden, status, body)
	require.NotContains(t, body, "library-instance")
	f.grant(t, model.PermissionFolder, "template-library-owner", model.PermissionView)
	status, body = f.get(t, "/api/v1/business/instances?template_id=linked-template")
	require.Equal(t, http.StatusOK, status, body)
	require.Contains(t, body, "library-instance")
}
