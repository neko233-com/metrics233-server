package localauth

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/neko233-com/metrics233-server/internal/identity"
	"github.com/neko233-com/metrics233-server/internal/instance"
	"github.com/neko233-com/metrics233-server/internal/server"
	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/stretchr/testify/require"
)

func TestNamespaceAPIsRequireLocalAuthenticationAndAcceptSessions(t *testing.T) {
	const endpoint = "/apis/notifications.alerting.grafana.app/v1beta1/namespaces/default/inhibitionrules"
	for _, legacyToken := range []string{"", "namespace-automation-233233"} {
		t.Run(fmt.Sprint(legacyToken != ""), func(t *testing.T) {
			handler, catalog := newHandlerWithAdmin(t, legacyToken)
			business, err := store.Open(filepath.Join(t.TempDir(), "control.db"))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, business.DB.Close()) })
			application := server.New(business, legacyToken, 30).Handler()
			anonymous := serve(handler, application, "GET", endpoint, "", nil)
			require.Equal(t, http.StatusUnauthorized, anonymous.Code, "namespace APIs must not inherit the local administrator fallback")
			login := serve(handler, application, "POST", "/login", `{"user":"admin","password":"correct-horse-233"}`, nil)
			require.Equal(t, http.StatusOK, login.Code)
			cookies := login.Result().Cookies()
			require.NotEmpty(t, cookies)
			response := serve(handler, application, "GET", endpoint, "", map[string]string{"Cookie": cookies[0].Name + "=" + cookies[0].Value})
			require.Equal(t, http.StatusOK, response.Code, "namespace APIs must receive the authenticated principal even with a workspace token configured")
			require.Contains(t, response.Body.String(), "InhibitionRuleList")
			_, err = catalog.CreateUser(t.Context(), identity.UserInput{Login: "namespace-viewer", Password: "viewer-password-233", Role: identity.RoleViewer, OrgID: 1})
			require.NoError(t, err)
			response = serve(handler, application, "GET", endpoint, "", basicHeaders("namespace-viewer", "viewer-password-233"))
			require.Equal(t, http.StatusOK, response.Code)
			response = serve(handler, application, "POST", endpoint, `{}`, basicHeaders("namespace-viewer", "viewer-password-233"))
			require.Equal(t, http.StatusForbidden, response.Code)
		})
	}
}

func basicHeaders(login, password string) map[string]string {
	return map[string]string{"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(login+":"+password))}
}

func TestPasswordFailuresShareLimitAcrossBasicAPIsAndLogin(t *testing.T) {
	handler, _ := newHandlerWithAdmin(t, "automation-token-233233")
	next, _ := nextHandler(t)
	for i := 0; i < 8; i++ {
		response := serve(handler, next, "GET", "/api/v1/stats", "", basicHeaders("admin", "deliberately-incorrect"))
		require.Equal(t, http.StatusUnauthorized, response.Code)
	}
	blocked := serve(handler, next, "GET", "/api/auth/me", "", basicHeaders("admin", "correct-horse-233"))
	require.Equal(t, http.StatusTooManyRequests, blocked.Code, "Basic authentication must not bypass password-attempt throttling")
	require.NotEmpty(t, blocked.Header().Get("Retry-After"))
	blocked = serve(handler, next, "POST", "/login", `{"user":"admin","password":"correct-horse-233"}`, nil)
	require.Equal(t, http.StatusTooManyRequests, blocked.Code, "form and Basic passwords must share the same failure budget")
	// Workspace automation tokens are independent of password failures.
	response := serve(handler, next, "GET", "/api/v1/stats", "", map[string]string{"Authorization": "Bearer automation-token-233233"})
	require.Equal(t, http.StatusOK, response.Code)
}

func TestSuccessfulBasicAuthenticationDoesNotConsumeFailureBudget(t *testing.T) {
	handler, _ := newHandlerWithAdmin(t, "")
	next, _ := nextHandler(t)
	for i := 0; i < 12; i++ {
		response := serve(handler, next, "GET", "/api/v1/stats", "", basicHeaders("admin", "correct-horse-233"))
		require.Equal(t, http.StatusOK, response.Code, "valid CLI requests must not consume the failure-only quota")
	}
	for i := 0; i < 7; i++ {
		response := serve(handler, next, "GET", "/api/v1/stats", "", basicHeaders("admin", "incorrect"))
		require.Equal(t, http.StatusUnauthorized, response.Code)
	}
	response := serve(handler, next, "POST", "/login", `{"user":"admin","password":"correct-horse-233"}`, nil)
	require.Equal(t, http.StatusOK, response.Code, "a successful form login clears prior Basic failures")
	for i := 0; i < 8; i++ {
		response = serve(handler, next, "POST", "/login", `{"user":"admin","password":"incorrect"}`, nil)
		require.Equal(t, http.StatusUnauthorized, response.Code)
	}
	response = serve(handler, next, "GET", "/api/v1/stats", "", basicHeaders("admin", "correct-horse-233"))
	require.Equal(t, http.StatusTooManyRequests, response.Code)
}

func TestTrustedProxyUsesRightmostUntrustedIPAcrossHeaderFields(t *testing.T) {
	_, trusted, err := net.ParseCIDR("10.0.0.0/24")
	require.NoError(t, err)
	handler := &Handler{trusted: []*net.IPNet{trusted}}
	r := httptest.NewRequest("POST", "/login", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Add("X-Forwarded-For", "203.0.113.99") // Untrusted incoming field.
	r.Header.Add("X-Forwarded-For", "198.51.100.23, 10.0.0.2")
	require.Equal(t, "198.51.100.23", handler.clientIP(r), "forwarded fields form one ordered chain")
	for i := 0; i < 10; i++ {
		r.Header["X-Forwarded-For"][0] = "203.0.113." + strconv.Itoa(i+1)
		require.Equal(t, "198.51.100.23", handler.clientIP(r), "an attacker must not rotate the limiter key through an earlier field")
	}
}

func TestAttemptLimiterBoundsUnexpiredClientState(t *testing.T) {
	limiter := newAttemptLimiter(8, 5*time.Minute)
	for i := 0; i < 4096; i++ {
		require.True(t, limiter.allow(fmt.Sprintf("client-%d", i)))
	}
	require.False(t, limiter.allow("overflow-client"), "new keys must fail closed when all bounded entries are active")
	require.Len(t, limiter.entries, 4096)
	// Existing callers retain their own remaining failure budget.
	require.True(t, limiter.allow("client-0"))
}

func TestConfiguredHTTPSOriginWorksBehindHTTPProxy(t *testing.T) {
	catalog := openedCatalog(t)
	bootstrapAdmin(t, catalog)
	authenticator, err := instance.NewHTTPAuthenticator(catalog, instance.HTTPAuthenticationConfig{
		CookieName: identity.SessionCookieName, PublicOrigin: "https://metrics.example.test",
	})
	require.NoError(t, err)
	handler, err := New(Config{Catalog: catalog, Authenticator: authenticator, SecureCookie: true})
	require.NoError(t, err)
	next, _ := nextHandler(t)
	request := httptest.NewRequest("POST", "http://metrics.example.test/login", strings.NewReader(`{"user":"admin","password":"correct-horse-233"}`))
	request.Header.Set("Origin", "https://metrics.example.test")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Forwarded-Proto", "arbitrary-attacker-value")
	response := httptest.NewRecorder()
	handler.Wrap(next).ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, "a configured public HTTPS origin must work over the proxy's HTTP upstream")
	var session string
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == identity.SessionCookieName {
			require.True(t, cookie.HttpOnly)
			require.True(t, cookie.Secure)
			session = cookie.Name + "=" + cookie.Value
		}
	}
	require.True(t, session != "")
	response = serve(handler, next, "POST", "http://metrics.example.test/api/v1/ingest", `{}`, map[string]string{"Origin": "https://metrics.example.test", "Cookie": session})
	require.Equal(t, http.StatusOK, response.Code)
	response = serve(handler, next, "POST", "http://metrics.example.test/api/v1/ingest", `{}`, map[string]string{"Origin": "https://foreign.invalid", "Cookie": session, "X-Forwarded-Proto": "https"})
	require.Equal(t, http.StatusForbidden, response.Code)
}

func TestConcurrentPasswordFailuresCannotRacePastQuota(t *testing.T) {
	handler, _ := newHandlerWithAdmin(t, "")
	next, _ := nextHandler(t)
	results := make(chan int, 24)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for i := 0; i < cap(results); i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			results <- serve(handler, next, "GET", "/api/v1/stats", "", basicHeaders("admin", "incorrect")).Code
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	counts := map[int]int{}
	for status := range results {
		counts[status]++
	}
	require.Equal(t, map[int]int{http.StatusUnauthorized: 8, http.StatusTooManyRequests: 16}, counts)
}

func TestRejectedCrossSiteLoginDoesNotSpendPasswordBudget(t *testing.T) {
	handler, _ := newHandlerWithAdmin(t, "")
	next, _ := nextHandler(t)
	for i := 0; i < 12; i++ {
		response := serve(handler, next, "POST", "/login", `{"user":"admin","password":"incorrect"}`, map[string]string{"Origin": "https://foreign.invalid"})
		require.Equal(t, http.StatusForbidden, response.Code)
	}
	response := serve(handler, next, "POST", "/login", `{"user":"admin","password":"correct-horse-233"}`, nil)
	require.Equal(t, http.StatusOK, response.Code)
}

func TestLoginCanonicalRedirectCannotResetFailureBudget(t *testing.T) {
	handler, _ := newHandlerWithAdmin(t, "")
	next, _ := nextHandler(t)
	for i := 0; i < 7; i++ {
		response := serve(handler, next, "POST", "/login", `{"user":"admin","password":"incorrect"}`, nil)
		require.Equal(t, http.StatusUnauthorized, response.Code)
	}
	redirect := serve(handler, next, "POST", "/unused/../login", `{}`, nil)
	require.Equal(t, http.StatusTemporaryRedirect, redirect.Code)
	response := serve(handler, next, "POST", "/login", `{"user":"admin","password":"incorrect"}`, nil)
	require.Equal(t, http.StatusUnauthorized, response.Code)
	response = serve(handler, next, "POST", "/login", `{"user":"admin","password":"correct-horse-233"}`, nil)
	require.Equal(t, http.StatusTooManyRequests, response.Code, "a pre-auth redirect must not clear failed-password state")
}

func TestSetupTokenReadersAreSafeDuringFirstAccountCreation(t *testing.T) {
	handler, _ := newHandler(t, "")
	token := handler.SetupToken()
	next, _ := nextHandler(t)
	done := make(chan struct{})
	var readers sync.WaitGroup
	for i := 0; i < 4; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-done:
					return
				default:
					_ = handler.SetupToken()
				}
			}
		}()
	}
	response := serve(handler, next, "POST", "/api/setup", fmt.Sprintf(`{"token":%q,"login":"admin","password":"correct-horse-233"}`, token), nil)
	close(done)
	readers.Wait()
	require.Equal(t, http.StatusOK, response.Code)
	require.True(t, handler.SetupToken() == "", "setup token was not retired")
}
