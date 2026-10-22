package hostagent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/stretchr/testify/require"
)

func TestBootstrapOriginsAndProtectedFile(t *testing.T) {
	cfg := model.HostBootstrap{Server: "https://example.com", HostID: "123e4567-e89b-12d3-a456-426614174000", Token: strings.Repeat("x", 43)}
	for _, server := range []string{"https://example.com", "http://127.0.0.1:7333", "http://[::1]:7333"} {
		cfg.Server = server
		require.NoError(t, ValidateBootstrap(cfg))
	}
	for _, server := range []string{"http://example.com", "https://user:password@example.com", "https://example.com/api", "https://example.com?token=anything", "file:///etc/passwd"} {
		cfg.Server = server
		require.Error(t, ValidateBootstrap(cfg))
	}
	path := filepath.Join(t.TempDir(), "bootstrap.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"server":"https://example.com","host_id":"123e4567-e89b-12d3-a456-426614174000","token":"`+strings.Repeat("x", 43)+`"}`), 0600))
	_, err := ReadBootstrap(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte(`{"server":"https://example.com"} {}`), 0600))
	_, err = ReadBootstrap(path)
	require.Error(t, err)
}

func TestAgentTransportDoesNotFollowRedirectOrLeakProviderErrors(t *testing.T) {
	var forwarded bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded = true }))
	defer destination.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 302) }))
	defer redirect.Close()
	cfg := model.HostBootstrap{Server: redirect.URL, HostID: "123e4567-e89b-12d3-a456-426614174000", Token: strings.Repeat("x", 43)}
	err := Exchange(context.Background(), NewClient(), cfg, "GET", "/configuration", nil, new(any))
	require.Error(t, err)
	require.False(t, forwarded)
	require.NotContains(t, err.Error(), cfg.Token)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("private-provider-error-password"))
	}))
	defer provider.Close()
	cfg.Server = provider.URL
	err = Exchange(context.Background(), NewClient(), cfg, "GET", "/configuration", nil, new(any))
	require.Error(t, err)
	require.NotContains(t, err.Error(), "password")
}
