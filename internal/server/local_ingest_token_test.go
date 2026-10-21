package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/neko233-com/metrics233-server/internal/identity"
	"github.com/neko233-com/metrics233-server/internal/instance"
	"github.com/neko233-com/metrics233-server/internal/localauth"
	"github.com/neko233-com/metrics233-server/internal/server"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestLocalAccountsAndIngestOnlyTokenRealHTTPAllFiveProtocols(t *testing.T) {
	db, _ := setup(t, "")
	catalog, err := identity.Open(filepath.Join(t.TempDir(), "identity.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, catalog.Close()) })
	_, err = catalog.BootstrapAdmin(t.Context(), identity.UserInput{Login: "admin", Password: "synthetic-admin-password-233"})
	require.NoError(t, err)
	_, err = catalog.CreateUser(t.Context(), identity.UserInput{Login: "viewer", Password: "synthetic-viewer-password-233", Role: identity.RoleViewer, OrgID: 1})
	require.NoError(t, err)
	authenticator, err := instance.NewHTTPAuthenticator(catalog, instance.HTTPAuthenticationConfig{CookieName: identity.SessionCookieName})
	require.NoError(t, err)
	auth, err := localauth.New(localauth.Config{Catalog: catalog, Authenticator: authenticator, SecureCookie: true})
	require.NoError(t, err)
	application := server.New(db, "", 15)
	require.NoError(t, application.ConfigureIngestToken("local", ingestOnlyFixtureToken))
	t.Cleanup(func() { application.Live.Close(); application.Plugins.Close() })
	endpoint := httptest.NewTLSServer(application.WithLocalIngestAuthentication(application.Handler(), auth.Wrap))
	defer endpoint.Close()
	client := endpoint.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	call := func(method, path, body, token string, cookie *http.Cookie) (int, []byte, []*http.Cookie) {
		r, err := http.NewRequest(method, endpoint.URL+path, strings.NewReader(body))
		require.NoError(t, err)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		response, err := client.Do(r)
		require.NoError(t, err)
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		return response.StatusCode, data, response.Cookies()
	}
	for _, token := range []string{"", "synthetic-invalid-token-233"} {
		status, _, _ := call("GET", "/api/v1/stats", "", token, nil)
		require.Equal(t, 401, status)
	}
	status, _, cookies := call("POST", "/login", `{"user":"admin","password":"synthetic-admin-password-233"}`, "", nil)
	require.Equal(t, 200, status)
	require.NotEmpty(t, cookies)
	adminCookie := cookies[0]
	require.True(t, adminCookie.HttpOnly)
	require.True(t, adminCookie.Secure)
	require.Equal(t, http.SameSiteLaxMode, adminCookie.SameSite)
	status, _, _ = call("GET", "/api/v1/stats", "", "", adminCookie)
	require.Equal(t, 200, status)
	remote, err := proto.Marshal(remoteWriteFixture(time.Now().UnixMilli()))
	require.NoError(t, err)
	otlp, err := protojson.Marshal(otlpFixture())
	require.NoError(t, err)
	for _, route := range []struct {
		path, kind, encoding string
		body                 []byte
		status               int
	}{
		{"/api/v1/ingest", "application/json", "", []byte(`{"samples":[{"name":"local_ingest_business_total","value":1}]}`), 200},
		{"/api/v1/write", "application/x-protobuf", "snappy", snappy.Encode(nil, remote), 204},
		{"/prometheus/api/v1/write", "application/x-protobuf", "snappy", snappy.Encode(nil, remote), 204},
		{"/v1/metrics", "application/json", "", otlp, 200},
		{"/otlp/v1/metrics", "application/json", "", otlp, 200},
	} {
		r, err := http.NewRequest("POST", endpoint.URL+route.path, bytes.NewReader(route.body))
		require.NoError(t, err)
		r.Header.Set("Authorization", "Bearer "+ingestOnlyFixtureToken)
		r.Header.Set("Content-Type", route.kind)
		r.Header.Set("Content-Encoding", route.encoding)
		r.Header.Set("X-Grafana-Org-Id", "2")
		response, err := client.Do(r)
		require.NoError(t, err)
		_, _ = io.Copy(io.Discard, response.Body)
		response.Body.Close()
		require.Equal(t, route.status, response.StatusCode, route.path)
	}
	view, err := db.ReportingSources(t.Context())
	require.NoError(t, err)
	require.Len(t, view.Sources, 1)
	require.EqualValues(t, 5, view.Sources[0].AcceptedBatches)
	for _, route := range []struct{ method, path string }{
		{"GET", "/api/v1/query?metric=local_ingest_business_total"}, {"GET", "/prometheus/api/v1/query?query=vector(1)"}, {"GET", "/api/v1/reporting-sources"}, {"PUT", "/api/v1/reporting-sources/policy"},
		{"POST", "/login"}, {"POST", "/api/setup"}, {"GET", "/api/auth/status"}, {"GET", "/api/auth/me"}, {"POST", "/api/auth/logout"}, {"POST", "/api/live/session"}, {"POST", "/api/live/publish/example"},
		{"GET", "/api/health"}, {"GET", "/metrics"}, {"GET", "/"}, {"GET", "/unknown-private-route"}, {"POST", "/unknown-private-route"}, {"GET", "/api/v1/ingest"}, {"POST", "/api/v1/ingest/"}, {"POST", "/api/v1/ingest/extra"},
	} {
		for _, cookie := range []*http.Cookie{nil, adminCookie} {
			status, _, resultCookies := call(route.method, route.path, `{"user":"admin","password":"synthetic-admin-password-233"}`, ingestOnlyFixtureToken, cookie)
			require.Equal(t, 403, status, route.method+" "+route.path)
			require.Empty(t, resultCookies)
		}
	}
	for _, headers := range [][]string{{"Bearer wrong"}, {ingestOnlyFixtureToken}, {"bearer " + ingestOnlyFixtureToken}, {"Bearer " + ingestOnlyFixtureToken, "Bearer " + ingestOnlyFixtureToken}, {"Bearer wrong", "Bearer " + ingestOnlyFixtureToken}, {"Bearer " + ingestOnlyFixtureToken, "Bearer wrong"}} {
		r, err := http.NewRequest("POST", endpoint.URL+"/api/v1/ingest", strings.NewReader(`{}`))
		require.NoError(t, err)
		for _, value := range headers {
			r.Header.Add("Authorization", value)
		}
		response, err := client.Do(r)
		require.NoError(t, err)
		response.Body.Close()
		require.Equal(t, 401, response.StatusCode)
	}
	// The reporting credential never authorizes the source-policy mutation, even
	// with an actual valid admin cookie attached. Admin sessions remain separate.
	policy, _ := json.Marshal(map[string]any{"version": view.Version, "allowlist_only": false, "address": view.Sources[0].Address, "action": "block"})
	status, _, _ = call("PUT", "/api/v1/reporting-sources/policy", string(policy), ingestOnlyFixtureToken, adminCookie)
	require.Equal(t, 403, status)
	status, _, _ = call("PUT", "/api/v1/reporting-sources/policy", string(policy), "", adminCookie)
	require.Equal(t, 200, status)
	status, _, _ = call("POST", "/api/v1/ingest", `{"samples":[{"name":"must_not_write_after_block","value":1}]}`, ingestOnlyFixtureToken, nil)
	require.Equal(t, 403, status)
	view, err = db.ReportingSources(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 5, view.Sources[0].AcceptedBatches)
	status, _, _ = call("POST", "/login", `{"user":"viewer","password":"synthetic-viewer-password-233"}`, "", nil)
	require.Equal(t, 200, status)
	status, _, _ = call("POST", "/api/auth/logout", "", "", adminCookie)
	require.Equal(t, 204, status)
	status, _, _ = call("GET", "/api/v1/stats", "", "", adminCookie)
	require.Equal(t, 401, status)
}

func TestLocalIngestPrincipalIsEditorScopedAndCustomAuthorizerCannotBeBypassed(t *testing.T) {
	db, _ := setup(t, "")
	application := server.New(db, "", 15)
	require.NoError(t, application.ConfigureIngestToken("local", ingestOnlyFixtureToken))
	t.Cleanup(func() { application.Live.Close(); application.Plugins.Close() })
	var seen identity.Principal
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		seen, ok = identity.FromContext(r.Context())
		require.True(t, ok)
		w.WriteHeader(204)
	})
	handler := application.WithLocalIngestAuthentication(inner, func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) })
	})
	r := httptest.NewRequest("POST", "/api/v1/ingest", nil)
	r.Header.Set("Authorization", "Bearer "+ingestOnlyFixtureToken)
	r.Header.Set("X-Grafana-Org-Id", "2")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	require.Equal(t, 204, w.Code)
	require.Equal(t, identity.RoleEditor, seen.Role)
	require.Equal(t, identity.TypeSystem, seen.Type)
	require.EqualValues(t, 1, seen.OrgID)
	require.False(t, seen.IsServerAdmin)
	require.ErrorIs(t, seen.RequireOrganization(2), identity.ErrForbidden)
	application.Authorizer = server.RequestAuthorizerFunc(func(*http.Request, server.ProtectedRoute) (identity.Principal, error) {
		t.Fatal("reporting bypass must not invoke/pretend to satisfy another provider")
		return identity.Principal{}, nil
	})
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	require.Equal(t, 403, w.Code)
	require.Error(t, application.ConfigureIngestToken("local", ingestOnlyFixtureToken))
}
