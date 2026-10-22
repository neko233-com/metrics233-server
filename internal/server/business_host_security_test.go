package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/neko233-com/metrics233-server/internal/identity"
	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/stretchr/testify/require"
)

func TestBusinessHostAssociationIsAdminOnlyWhileResourceProjectionIsReadable(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "control.db"))
	require.NoError(t, err)
	defer db.DB.Close()
	instance := model.BusinessIdentity{Service: "fixture", Role: "logic", ServerID: "1", Instance: "fixture"}
	require.NoError(t, db.Ingest(context.Background(), []model.Sample{{Name: "game_players_online", Value: 1, Labels: instance.Labels()}}))
	s := New(db, "synthetic-admin-token", 15)
	handler := s.Handler()
	defer s.Plugins.Close()
	defer s.Live.Close()
	request := func(method, path string, role identity.Role) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"version": 0, "kind": "", "id": ""})
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r = r.WithContext(identity.WithPrincipal(r.Context(), identity.Principal{OrgID: 1, Type: identity.TypeUser, ID: 1, Login: "fixture", Role: role}))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	path := "/api/v1/business/instances/" + store.BusinessInstanceID(instance)
	for _, role := range []identity.Role{identity.RoleViewer, identity.RoleEditor} {
		require.Equal(t, 403, request("PUT", path+"/host", role).Code)
		require.Equal(t, 403, request("GET", "/api/v1/business/host-options", role).Code)
		require.Equal(t, 200, request("GET", path+"/dashboard", role).Code)
	}
	require.Equal(t, 200, request("PUT", path+"/host", identity.RoleAdmin).Code)
}
