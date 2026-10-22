package hostagent

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestSSHInstallerPinsBeforeAuthenticationAndRejectsMissingArtifact(t *testing.T) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(key)
	require.NoError(t, err)
	var authenticated atomic.Int32
	config := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		authenticated.Add(1)
		if string(password) != "synthetic-password" {
			return nil, fmt.Errorf("authentication rejected")
		}
		return nil, nil
	}}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				_, channels, requests, err := ssh.NewServerConn(conn, config)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(requests)
				for channel := range channels {
					c, requests, err := channel.Accept()
					if err != nil {
						return
					}
					for request := range requests {
						if request.Type == "exec" {
							_ = request.Reply(true, nil)
							_, _ = io.WriteString(c, "Linux\nx86_64\n0\n")
							_, _ = c.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
							c.Close()
							break
						}
						_ = request.Reply(false, nil)
					}
				}
			}()
		}
	}()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	var portNumber int
	_, err = fmt.Sscan(port, &portNumber)
	require.NoError(t, err)
	h := model.ManagedHost{ID: "123e4567-e89b-12d3-a456-426614174000", Name: "Synthetic SSH", Address: "127.0.0.1", Port: portNumber, User: "metrics", HostKey: "SHA256:" + strings.Repeat("x", 43), Target: model.Target{Name: "mysql", Kind: "mysql", URL: "mysql://localhost:3306", IntervalSeconds: 30}}
	bootstrap := model.HostBootstrap{Server: "https://example.com", HostID: h.ID, Token: strings.Repeat("x", 43)}
	secrets := map[string]string{"ssh_password": "synthetic-password"}
	_, err = Deploy(context.Background(), h, secrets, bootstrap, t.TempDir())
	require.ErrorContains(t, err, "pinned host key")
	require.Zero(t, authenticated.Load(), "untrusted peer must never receive password authentication")
	fingerprint, err := Fingerprint(context.Background(), h)
	require.NoError(t, err)
	require.Equal(t, ssh.FingerprintSHA256(signer.PublicKey()), fingerprint)
	require.Zero(t, authenticated.Load(), "fingerprint discovery is a handshake, not authentication")
	h.HostKey = fingerprint
	_, err = Deploy(context.Background(), h, secrets, bootstrap, t.TempDir())
	require.ErrorContains(t, err, "artifact is unavailable")
	require.EqualValues(t, 1, authenticated.Load())
}

func TestInstallerScriptsHaveValidShellSyntaxAndScopedServicePaths(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		candidate := "C:/Program Files/Git/bin/bash.exe"
		if _, e := os.Stat(candidate); e != nil {
			t.Skip("POSIX shell parser unavailable")
		}
		sh = candidate
	}
	for _, platform := range []string{"darwin", "linux"} {
		t.Run(platform, func(t *testing.T) {
			checksum := hex.EncodeToString(make([]byte, 32))
			script := installerScript("123e4567-e89b-12d3-a456-426614174000", platform, checksum, ".metrics233-install-synthetic")
			path := filepath.Join(t.TempDir(), "install.sh")
			require.NoError(t, os.WriteFile(path, []byte(script), 0600))
			output, err := exec.Command(sh, "-n", path).CombinedOutput()
			require.NoError(t, err, string(output))
			require.Contains(t, script, "trap rollback EXIT")
			require.Contains(t, script, "chown \"$service_user\"")
			require.NotContains(t, script, "User=nobody")
			require.NotContains(t, script, "rm -rf")
			require.NotContains(t, script, "curl ")
			require.NotContains(t, script, "wget ")
		})
	}
}

func TestSudoCredentialIsNeverExecutedWithCachedOrUncachedAuthentication(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		sh = "C:/Program Files/Git/bin/bash.exe"
		if _, err = os.Stat(sh); err != nil {
			t.Skip("POSIX shell unavailable")
		}
	}
	root := t.TempDir()
	fake := filepath.Join(root, "sudo-fixture.sh")
	fixture := `#!/bin/sh
case "$*" in
  *-v) printf '%s' "$PPID" > "$METRICS233_FAKE_SUDO_PARENT"
       if test "$METRICS233_FAKE_SUDO_CACHED" = 1; then exit 0; fi
       IFS= read -r value; test "$value" = 'printf PASSWORD_EXECUTED' ;;
  *) test "$(cat "$METRICS233_FAKE_SUDO_PARENT")" = "$PPID" || exit 1
     exec ` + "'" + filepath.ToSlash(sh) + "'" + ` -s ;;
esac
`
	require.NoError(t, os.WriteFile(fake, []byte(fixture), 0755))
	command := strings.ReplaceAll(sudoShellCommand(), "/usr/bin/sudo", "'"+filepath.ToSlash(fake)+"'")
	shells := []string{sh}
	if zsh, err := exec.LookPath("zsh"); err == nil {
		shells = append(shells, zsh)
	}
	for _, shell := range shells {
		for _, cached := range []string{"0", "1"} {
			cmd := exec.Command(shell, "-c", command)
			cmd.Env = append(os.Environ(), "METRICS233_FAKE_SUDO_CACHED="+cached, "METRICS233_FAKE_SUDO_PARENT="+filepath.ToSlash(filepath.Join(root, "parent")))
			cmd.Stdin = strings.NewReader("printf PASSWORD_EXECUTED\nprintf INSTALLER_RAN\n")
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s cached=%s: %s", shell, cached, string(output))
			require.Equal(t, "INSTALLER_RAN", string(output), "sudo cache must preserve its PPID ticket without turning credentials into code")
		}
	}
}
