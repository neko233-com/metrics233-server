package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/stretchr/testify/require"
)

func TestManagedMySQLDashboardCreatePreservesLaterCustomPanels(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "control.db"))
	require.NoError(t, err)
	defer db.DB.Close()
	ctx := context.Background()
	h, err := db.SaveManagedHost(ctx, model.ManagedHost{Name: "Fixture MySQL", Address: "127.0.0.1", Port: 22, User: "metrics", Target: model.Target{Name: "mysql", Kind: "mysql", URL: "mysql://localhost:3306", IntervalSeconds: 30}}, "fixture")
	require.NoError(t, err)
	handler := New(db, "", 15).Handler()
	path := "/api/v1/managed-hosts/" + h.ID + "/dashboard"
	create := func() model.Dashboard {
		r := httptest.NewRequest("POST", path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		require.Equal(t, 200, w.Code, w.Body.String())
		var d model.Dashboard
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &d))
		return d
	}
	d := create()
	require.Len(t, d.Panels, 18)
	require.Contains(t, d.Panels[0].Expr, h.ID)
	d.Panels[0].Title = "My custom availability"
	d.Panels = d.Panels[:3]
	d, err = db.SaveDashboard(ctx, d)
	require.NoError(t, err)
	again := create()
	require.Equal(t, d.UpdatedAt, again.UpdatedAt)
	require.Equal(t, d.Panels, again.Panels)
}
