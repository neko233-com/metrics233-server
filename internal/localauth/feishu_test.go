package localauth

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/neko233-com/metrics233-server/internal/identity"
	"github.com/neko233-com/metrics233-server/internal/server"
	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/stretchr/testify/require"
)

func TestFeishuContactPointLocalAccountRoleBoundary(t *testing.T) {
	auth, catalog := newHandlerWithAdmin(t, "")
	business, err := store.Open(filepath.Join(t.TempDir(), "control.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, business.DB.Close()) })
	application := server.New(business, "", 30).Handler()
	for _, role := range []identity.Role{identity.RoleViewer, identity.RoleEditor} {
		_, err := catalog.CreateUser(t.Context(), identity.UserInput{Login: string(role), Password: "synthetic-password-233", Role: role, OrgID: 1})
		require.NoError(t, err)
	}
	body := `{"uid":"feishu-role","name":"Feishu role fixture","type":"feishu","url":"https://open.feishu.cn/open-apis/bot/v2/hook/synthetic-role-fixture-only","signing_secret":"synthetic-role-secret"}`
	for _, role := range []identity.Role{identity.RoleViewer, identity.RoleEditor} {
		response := serve(auth, application, "POST", "/api/v1/alerting/contact-points", body, basicHeaders(string(role), "synthetic-password-233"))
		require.Equal(t, http.StatusForbidden, response.Code)
	}
	admin := basicHeaders("admin", "correct-horse-233")
	created := serve(auth, application, "POST", "/api/v1/alerting/contact-points", body, admin)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	for _, role := range []identity.Role{identity.RoleViewer, identity.RoleEditor} {
		headers := basicHeaders(string(role), "synthetic-password-233")
		response := serve(auth, application, "GET", "/api/v1/alerting/contact-points/feishu-role", "", headers)
		require.Equal(t, http.StatusOK, response.Code)
		require.NotContains(t, response.Body.String(), "synthetic-role-secret")
		require.NotContains(t, response.Body.String(), "synthetic-role-fixture-only")
		for _, mutation := range []struct{ method, path string }{
			{"PUT", "/api/v1/alerting/contact-points/feishu-role"},
			{"DELETE", "/api/v1/alerting/contact-points/feishu-role"},
			{"POST", "/api/v1/alerting/contact-points/feishu-role/test"},
			{"POST", "/api/v1/alerting/notifications/1/retry"},
		} {
			response := serve(auth, application, mutation.method, mutation.path, `{}`, headers)
			require.Equal(t, http.StatusForbidden, response.Code)
		}
	}
	queued := serve(auth, application, "POST", "/api/v1/alerting/contact-points/feishu-role/test", "", admin)
	require.Equal(t, http.StatusAccepted, queued.Code)
}
