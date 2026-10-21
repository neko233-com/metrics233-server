package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/neko233-com/metrics233-server/internal/identity"
	"github.com/stretchr/testify/require"
)

func TestInteractiveBootstrapReadsAndConfirmsHiddenPasswordOnly(t *testing.T) {
	_, boundaryErr := interactiveBootstrapInput("admin", "", "", func(string) ([]byte, error) { return []byte("Abc123!x"), nil })
	require.NoError(t, boundaryErr, "interactive bootstrap must use the same eight-byte minimum as identity credentials")
	_, boundaryErr = interactiveBootstrapInput("admin", "", "", func(string) ([]byte, error) { return []byte("short7!"), nil })
	require.EqualError(t, boundaryErr, "bootstrap_password_length_invalid")
	prompts := []string{}
	values := [][]byte{[]byte("synthetic-bootstrap-password-233"), []byte("synthetic-bootstrap-password-233")}
	data, err := interactiveBootstrapInput("admin", "Operator", "", func(prompt string) ([]byte, error) {
		prompts = append(prompts, prompt)
		next := values[0]
		values = values[1:]
		return next, nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{"Password: ", "Confirm password: "}, prompts)
	var input bootstrapAdminInput
	require.NoError(t, json.Unmarshal(data, &input))
	require.Equal(t, "admin", input.Login)
	require.Equal(t, "synthetic-bootstrap-password-233", input.Password)
	_, err = interactiveBootstrapInput("admin", "", "", func(prompt string) ([]byte, error) {
		if prompt == "Password: " {
			return []byte("synthetic-bootstrap-password-233"), nil
		}
		return []byte("mismatched-password-233"), nil
	})
	require.EqualError(t, err, "bootstrap_password_confirmation_mismatch")
	_, err = interactiveBootstrapInput("admin", "", "", func(string) ([]byte, error) { return nil, errors.New("synthetic-secret-source-error") })
	require.EqualError(t, err, "bootstrap_password_input_failed")
}

func TestBootstrapOfflineCreatesAdminOnceWithoutControlStoreAndNeverReturnsSecrets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Linux provisioning filesystem permissions; exercised in Linux fixture")
	}
	parent := t.TempDir()
	require.NoError(t, os.Chmod(parent, 0700))
	path := filepath.Join(parent, "identity.db")
	const password = "synthetic-bootstrap-password-233"
	input := `{"login":"admin","password":"` + password + `"}`
	var output bytes.Buffer
	require.NoError(t, bootstrapAdminFromInput(context.Background(), path, strings.NewReader(input), &output))
	require.NotContains(t, output.String(), password)
	require.NotContains(t, output.String(), "admin")
	require.NotContains(t, output.String(), path)
	var result map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &result))
	require.Equal(t, true, result["initialized"])
	require.Equal(t, "Admin", result["role"])
	require.Len(t, result, 4)
	catalog, err := identity.Open(path)
	require.NoError(t, err)
	principal, err := catalog.AuthenticatePassword(context.Background(), "admin", password, 1)
	require.NoError(t, err)
	require.Equal(t, identity.RoleAdmin, principal.Role)
	require.True(t, principal.IsServerAdmin)
	require.NoError(t, catalog.Close())
	output.Reset()
	err = bootstrapAdminFromInput(context.Background(), path, strings.NewReader(`{"login":"another","password":"different-bootstrap-password-233"}`), &output)
	require.EqualError(t, err, "bootstrap_already_initialized")
	require.Empty(t, output.String())
	for _, name := range []string{"control.db", "metrics.db", "secrets.key"} {
		_, err := os.Stat(filepath.Join(parent, name))
		require.True(t, os.IsNotExist(err))
	}
}

func TestBootstrapInvalidInputNeverCreatesCatalog(t *testing.T) {
	for _, input := range []string{`{"login":"admin","password":"short"}`, `{"login":"admin","password":"synthetic-bootstrap-password-233","role":"Admin"}`, `{"login":"admin","password":"synthetic-bootstrap-password-233"} {}`, `{`, strings.Repeat("x", bootstrapInputLimit+1)} {
		path := filepath.Join(t.TempDir(), "identity.db")
		var output bytes.Buffer
		err := bootstrapAdminFromInput(context.Background(), path, strings.NewReader(input), &output)
		require.Error(t, err)
		require.NotContains(t, err.Error(), input)
		require.Empty(t, output.String())
		_, err = os.Stat(path)
		require.True(t, os.IsNotExist(err))
	}
}

func TestBootstrapRequiresProtectedAbsoluteNonLinkPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Linux provisioning path and mode boundary")
	}
	parent := t.TempDir()
	require.NoError(t, os.Chmod(parent, 0700))
	path := filepath.Join(parent, "identity.db")
	require.NoError(t, validateBootstrapPath(path))
	require.Error(t, validateBootstrapPath("identity.db"))
	require.NoError(t, os.Chmod(parent, 0750))
	require.Error(t, validateBootstrapPath(path))
	require.NoError(t, os.Chmod(parent, 0700))
	target := filepath.Join(parent, "target.db")
	require.NoError(t, os.WriteFile(target, []byte("synthetic existing file"), 0600))
	require.NoError(t, os.Symlink(target, path))
	require.Error(t, validateBootstrapPath(path))
	require.NoError(t, os.Remove(path))
	require.NoError(t, os.WriteFile(path+"-wal", []byte("synthetic unsafe sidecar"), 0644))
	require.Error(t, validateBootstrapPath(path))
}

func TestBootstrapCommandRejectsPasswordArgsBeforeOpeningCatalog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "must-not-create.db")
	for _, args := range [][]string{{"--identity-db", path, "--password", "synthetic-private-password"}, {"--identity-db", path, "--file", "credential.json"}, {"--identity-db", path, "--login", "admin", "--file", "-"}} {
		err := bootstrapAdminCommand(args)
		require.EqualError(t, err, "bootstrap_invalid_arguments")
		require.NotContains(t, err.Error(), "synthetic-private")
		_, err = os.Stat(path)
		require.True(t, os.IsNotExist(err))
	}
}
