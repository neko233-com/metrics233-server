package server_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neko233-com/metrics233-server/internal/identity"
	"github.com/neko233-com/metrics233-server/internal/instance"
	"github.com/neko233-com/metrics233-server/internal/localauth"
	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/server"
	"github.com/stretchr/testify/require"
)

func TestFeishuPreviewRealLocalHTTPRolesAndIngestToken(t *testing.T) {
	db, _ := setup(t, "")
	catalog, err := identity.Open(filepath.Join(t.TempDir(), "identity.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, catalog.Close()) })
	const password = "synthetic-preview-account-password-233"
	_, err = catalog.BootstrapAdmin(t.Context(), identity.UserInput{Login: "admin", Password: password})
	require.NoError(t, err)
	for _, entry := range []struct {
		login string
		role  identity.Role
	}{{"viewer", identity.RoleViewer}, {"editor", identity.RoleEditor}} {
		_, err = catalog.CreateUser(t.Context(), identity.UserInput{Login: entry.login, Password: password, Role: entry.role, OrgID: 1})
		require.NoError(t, err)
	}
	authenticator, err := instance.NewHTTPAuthenticator(catalog, instance.HTTPAuthenticationConfig{CookieName: identity.SessionCookieName})
	require.NoError(t, err)
	auth, err := localauth.New(localauth.Config{Catalog: catalog, Authenticator: authenticator, SecureCookie: true})
	require.NoError(t, err)
	app := server.New(db, "", 15)
	require.NoError(t, app.ConfigureIngestToken("local", ingestOnlyFixtureToken))
	t.Cleanup(func() { app.Live.Close(); app.Plugins.Close() })
	endpoint := httptest.NewTLSServer(app.WithLocalIngestAuthentication(app.Handler(), auth.Wrap))
	defer endpoint.Close()
	for _, entry := range []struct {
		login, token string
		want         int
	}{{"", "", 401}, {"viewer", "", 403}, {"editor", "", 403}, {"admin", "", 200}, {"", ingestOnlyFixtureToken, 403}} {
		r, err := http.NewRequest(http.MethodPost, endpoint.URL+"/api/v1/alerting/contact-points/preview", strings.NewReader(`{}`))
		require.NoError(t, err)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", endpoint.URL)
		if entry.login != "" {
			r.SetBasicAuth(entry.login, password)
		}
		if entry.token != "" {
			r.Header.Set("Authorization", "Bearer "+entry.token)
		}
		response, err := endpoint.Client().Do(r)
		require.NoError(t, err)
		_, _ = io.Copy(io.Discard, response.Body)
		response.Body.Close()
		require.Equal(t, entry.want, response.StatusCode)
	}
	var count int
	require.NoError(t, db.DB.QueryRow(`SELECT count(*) FROM alert_notification_outbox`).Scan(&count))
	require.Zero(t, count)
}

func TestFeishuPreviewRendersDraftSavedTemplatesWithoutQueueOrSecrets(t *testing.T) {
	const token = "synthetic-management-preview-token"
	db, handler := setup(t, token)
	_, err := db.SaveNotificationTemplate(context.Background(), model.NotificationTemplate{Name: "card-title", Template: `{{define "card.title"}}Host {{.CommonLabels.instance}} {{.Status}}{{end}}`})
	require.NoError(t, err)
	for _, status := range []string{"firing", "resolved"} {
		body, _ := json.Marshal(map[string]string{"status": status, "title_template": `{{template "card.title" .}}`, "message_template": `{{ range .Alerts }}{{.Labels.resource}}={{.Values.A}} for {{.Rule.ForSeconds}}s; {{end}}`})
		response := call(handler, "POST", "/api/v1/alerting/contact-points/preview", string(body), token, "")
		require.Equal(t, 200, response.Code, response.Body.String())
		var result struct {
			Format  string
			Sent    bool
			Bytes   int
			Message map[string]any
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
		require.False(t, result.Sent)
		require.Equal(t, "interactive", result.Format)
		require.LessOrEqual(t, result.Bytes, 20*1024)
		require.Contains(t, response.Body.String(), "Host example-host "+status)
		require.NotContains(t, response.Body.String(), "open-apis/bot")
		require.NotContains(t, response.Body.String(), `"sign"`)
	}
	for _, body := range []string{`{"title_template":"{{"}`, `{"status":"invalid"}`, `{"feishu_format":"raw-json"}`, `{"url":"https://untrusted.example"}`} {
		require.Equal(t, 400, call(handler, "POST", "/api/v1/alerting/contact-points/preview", body, token, "").Code)
	}
	response := call(handler, "POST", "/api/v1/alerting/contact-points/preview", `{"feishu_format":"text"}`, token, "")
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"msg_type":"text"`)
	var count int
	require.NoError(t, db.DB.QueryRow(`SELECT count(*) FROM alert_notification_outbox`).Scan(&count))
	require.Zero(t, count)
}

func TestFeishuFormatPersistedAndMalformedTemplateRejected(t *testing.T) {
	const token = "synthetic-management-format-token"
	_, handler := setup(t, token)
	for _, format := range []string{"invalid", "markdown"} {
		body, _ := json.Marshal(map[string]string{"name": "Format", "type": "feishu", "url": "https://open.feishu.cn/open-apis/bot/v2/hook/synthetic-format", "feishu_format": format})
		require.Equal(t, 400, call(handler, "POST", "/api/v1/alerting/contact-points", string(body), token, "").Code)
	}
	for _, format := range []string{"interactive", "text"} {
		body, _ := json.Marshal(map[string]string{"name": "Format", "type": "feishu", "url": "https://open.feishu.cn/open-apis/bot/v2/hook/synthetic-format", "feishu_format": format, "title_template": "{{ .Status }}"})
		response := call(handler, "POST", "/api/v1/alerting/contact-points", string(body), token, "")
		require.Equal(t, 201, response.Code, response.Body.String())
		var point model.ContactPoint
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &point))
		require.Equal(t, format, point.FeishuFormat)
		get := call(handler, "GET", "/api/v1/alerting/contact-points/"+point.UID, "", token, "")
		require.Equal(t, 200, get.Code)
		require.Contains(t, get.Body.String(), `"feishu_format":"`+format+`"`)
	}
}
