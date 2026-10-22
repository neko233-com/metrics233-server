package hostagent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	model "github.com/neko233-com/metrics233-server/internal/telemetry"
	"golang.org/x/crypto/ssh"
)

func sshConnection(ctx context.Context, h model.ManagedHost, secrets map[string]string, inspect bool) (*ssh.Client, string, error) {
	var auth []ssh.AuthMethod
	if password := secrets["ssh_password"]; password != "" {
		auth = append(auth, ssh.Password(password))
	}
	if key := secrets["ssh_private_key"]; key != "" {
		signer, err := ssh.ParsePrivateKey([]byte(key))
		if err != nil {
			return nil, "", errors.New("invalid SSH private key")
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}
	fingerprint := ""
	config := &ssh.ClientConfig{User: h.User, Auth: auth, Timeout: 8 * time.Second, HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
		fingerprint = ssh.FingerprintSHA256(key)
		if inspect {
			return errors.New("fingerprint inspection complete")
		}
		if h.HostKey == "" || fingerprint != h.HostKey {
			return errors.New("SSH host key mismatch")
		}
		return nil
	}}
	conn, err := (&net.Dialer{Timeout: 8 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(h.Address, fmt.Sprint(h.Port)))
	if err != nil {
		return nil, "", errors.New("SSH target unreachable from this executor")
	}
	deadline := time.Now().Add(60 * time.Second)
	if end, ok := ctx.Deadline(); ok && end.Before(deadline) {
		deadline = end
	}
	_ = conn.SetDeadline(deadline)
	stopCancel := context.AfterFunc(ctx, func() { _ = conn.Close() })
	connection, channels, requests, err := ssh.NewClientConn(conn, h.Address, config)
	if err != nil {
		stopCancel()
		conn.Close()
		if inspect && fingerprint != "" {
			return nil, fingerprint, nil
		}
		return nil, "", errors.New("SSH authentication or pinned host key verification failed")
	}
	return ssh.NewClient(connection, channels, requests), fingerprint, nil
}

// Fingerprint performs only the handshake. It never authenticates with a
// password/key; the observed value must be verified before saving the pin.
func Fingerprint(ctx context.Context, h model.ManagedHost) (string, error) {
	_, key, err := sshConnection(ctx, h, nil, true)
	return key, err
}

type limitedOutput struct{ bytes.Buffer }

func (b *limitedOutput) Write(data []byte) (int, error) {
	if b.Len()+len(data) > 4096 {
		return 0, errors.New("SSH output budget exceeded")
	}
	return b.Buffer.Write(data)
}

func sshRun(client *ssh.Client, script string, input io.Reader) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", errors.New("SSH session unavailable")
	}
	defer session.Close()
	var stdout limitedOutput
	session.Stdout = &stdout
	session.Stderr = io.Discard
	session.Stdin = input
	if session.Run(script) != nil {
		return "", errors.New("remote operation failed; previous installation retained where possible")
	}
	return strings.TrimSpace(stdout.String()), nil
}

type Installation struct {
	OS    string `json:"os"`
	Arch  string `json:"arch"`
	SHA   string `json:"sha256"`
	State string `json:"state"`
}

// Deploy accepts only a server-owned artifact directory and fixed installer.
// Hostnames/users/secrets are never interpolated into remote shell commands.
func Deploy(ctx context.Context, h model.ManagedHost, secrets map[string]string, bootstrap model.HostBootstrap, artifacts string) (Installation, error) {
	var result Installation
	if err := h.Validate(); err != nil {
		return result, err
	}
	if _, err := uuid.Parse(h.ID); err != nil {
		return result, errors.New("invalid host identity")
	}
	if err := ValidateBootstrap(bootstrap); err != nil {
		return result, err
	}
	if h.ID != bootstrap.HostID {
		return result, errors.New("bootstrap host mismatch")
	}
	client, _, err := sshConnection(ctx, h, secrets, false)
	if err != nil {
		return result, err
	}
	defer client.Close()
	info, err := sshRun(client, "uname -s; uname -m; id -u", nil)
	if err != nil {
		return result, err
	}
	parts := strings.Fields(info)
	if len(parts) != 3 {
		return result, errors.New("unsupported target platform")
	}
	result.OS = strings.ToLower(parts[0])
	result.Arch = parts[1]
	if result.Arch == "x86_64" {
		result.Arch = "amd64"
	}
	if result.Arch == "aarch64" {
		result.Arch = "arm64"
	}
	if (result.OS != "linux" && result.OS != "darwin") || (result.Arch != "amd64" && result.Arch != "arm64") {
		return result, errors.New("automatic deployment supports Linux/macOS amd64/arm64")
	}
	if parts[2] != "0" && !h.Sudo {
		return result, errors.New("system service installation requires root or explicit sudo permission")
	}
	if artifacts == "" {
		return result, errors.New("collector artifacts are unavailable")
	}
	path := filepath.Join(artifacts, "metrics233-collector-"+result.OS+"-"+result.Arch)
	stat, err := os.Lstat(path)
	if err != nil || !stat.Mode().IsRegular() || stat.Size() < 1 || stat.Size() > 64<<20 {
		return result, errors.New("verified collector artifact is unavailable for this platform")
	}
	binary, err := os.ReadFile(path)
	if err != nil {
		return result, errors.New("cannot read collector artifact")
	}
	sum := sha256.Sum256(binary)
	result.SHA = hex.EncodeToString(sum[:])
	// A server administrator supplies immutable hashes beside artifacts. An
	// arbitrary file placed in this directory is never an installable artifact.
	manifest, err := os.ReadFile(path + ".sha256")
	if err != nil || strings.TrimSpace(string(manifest)) != result.SHA {
		return result, errors.New("collector artifact checksum mismatch")
	}
	stage := ".metrics233-install-" + uuid.NewString()
	if _, err = sshRun(client, "umask 077; mkdir \"$HOME/"+stage+"\"", nil); err != nil {
		return result, err
	}
	defer sshRun(client, "rm -f \"$HOME/"+stage+"/collector\" \"$HOME/"+stage+"/bootstrap.json\"; rmdir \"$HOME/"+stage+"\"", nil)
	if _, err = sshRun(client, "umask 077; cat > \"$HOME/"+stage+"/collector\"", bytes.NewReader(binary)); err != nil {
		return result, err
	}
	encoded, err := json.Marshal(bootstrap)
	if err != nil {
		return result, errors.New("cannot encode protected bootstrap")
	}
	if _, err = sshRun(client, "umask 077; cat > \"$HOME/"+stage+"/bootstrap.json\"", bytes.NewReader(encoded)); err != nil {
		return result, err
	}
	script := installerScript(h.ID, result.OS, result.SHA, stage)
	command := "/bin/sh -s"
	var input io.Reader = strings.NewReader(script)
	if parts[2] != "0" {
		if strings.ContainsAny(secrets["ssh_password"], "\r\n\x00") {
			return result, errors.New("sudo requires a single-line credential or configured noninteractive sudo")
		}
		// Always consume the credential ourselves. With cached/NOPASSWD sudo,
		// blindly prepending a password to shell stdin would execute it as code.
		command = sudoShellCommand()
		input = strings.NewReader(secrets["ssh_password"] + "\n" + script)
	}
	if _, err = sshRun(client, command, input); err != nil {
		return result, err
	}
	result.State = "installed_awaiting_heartbeat"
	return result, nil
}

func sudoShellCommand() string {
	// Keep the login shell alive for both sudo calls. zsh otherwise tail-execs
	// the second sudo, changing its parent and invalidating a no-TTY PPID ticket.
	return "IFS= read -r metrics233_sudo_password; printf '%s\\n' \"$metrics233_sudo_password\" | /usr/bin/sudo -S -p '' -v >/dev/null 2>&1 && unset metrics233_sudo_password && /usr/bin/sudo -n /bin/sh -s; metrics233_status=$?; unset metrics233_sudo_password; exit \"$metrics233_status\""
}

func installerScript(id, platform, checksum, stage string) string {
	// All four arguments are internal UUID/platform/hash values, never addresses,
	// usernames, passwords or arbitrary command text from a request.
	base := "/opt/metrics233-collector/" + id
	label := "com.metrics233.collector." + id
	prelude := `set -eu
umask 077
PATH=/usr/sbin:/usr/bin:/sbin:/bin
LC_ALL=C
export PATH LC_ALL
test "$(id -u)" = 0
stage="$HOME/` + stage + `"
# sudo may change HOME; recover the invoking user's exact home via the account
# database rather than accepting a path from the API.
if test -n "${SUDO_USER:-}"; then
  if test "$(uname -s)" = Darwin; then original_home=$(dscl . -read "/Users/$SUDO_USER" NFSHomeDirectory | cut -d ' ' -f2-)
  else original_home=$(getent passwd "$SUDO_USER" | cut -d: -f6); fi
  test -n "$original_home"; stage="$original_home/` + stage + `"
fi
base='` + base + `'
test ! -L /opt; mkdir -p /opt; test "$(ls -dn /opt | awk '{print $3}')" = 0
for d in /opt/metrics233-collector "$base" "$base/releases"; do test ! -L "$d"; mkdir -p "$d"; test "$(ls -dn "$d" | awk '{print $3}')" = 0; chmod 755 "$d"; done
release="$base/releases/` + checksum + `"
test ! -L "$release"; mkdir -p "$release"; test "$(ls -dn "$release" | awk '{print $3}')" = 0; chmod 755 "$release"
test ! -L "$release/collector"; test ! -L "$base/bootstrap.json"
cp "$stage/collector" "$release/collector.new"
`
	verify := "actual=$(sha256sum \"$release/collector.new\" | cut -d ' ' -f1)\n"
	if platform == "darwin" {
		verify = "actual=$(shasum -a 256 \"$release/collector.new\" | cut -d ' ' -f1)\n"
	}
	account := `service_user='metrics233-collector'
if ! id "$service_user" >/dev/null 2>&1; then useradd --system --user-group --no-create-home --home-dir /var/empty --shell /usr/sbin/nologin "$service_user"; fi
test "$(getent passwd "$service_user" | cut -d: -f6)" = /var/empty
test "$(id -u "$service_user")" -gt 0
test "$(id -gn "$service_user")" = "$service_user"
case "$(getent passwd "$service_user" | cut -d: -f7)" in */nologin|*/false) ;; *) exit 1;; esac
`
	if platform == "darwin" {
		account = `service_user='_metrics233'
if ! dscl . -read /Users/_metrics233 UniqueID >/dev/null 2>&1; then
  test -z "$(dscl . -list /Users UniqueID | awk '$2==52033 {print $1}')"
  test -z "$(dscl . -list /Groups PrimaryGroupID | awk '$2==52033 {print $1}')"
  dscl . -create /Groups/_metrics233
  dscl . -create /Groups/_metrics233 PrimaryGroupID 52033
  dscl . -create /Users/_metrics233
  dscl . -create /Users/_metrics233 UniqueID 52033
  dscl . -create /Users/_metrics233 PrimaryGroupID 52033
  dscl . -create /Users/_metrics233 UserShell /usr/bin/false
  dscl . -create /Users/_metrics233 NFSHomeDirectory /var/empty
  dscl . -create /Users/_metrics233 RealName 'Metrics233 Collector'
  dscl . -create /Users/_metrics233 IsHidden 1
  dscl . -create /Users/_metrics233 Password '*'
fi
test "$(dscl . -read /Users/_metrics233 UserShell | cut -d ' ' -f2-)" = /usr/bin/false
test "$(dscl . -read /Users/_metrics233 NFSHomeDirectory | cut -d ' ' -f2-)" = /var/empty
test "$(dscl . -read /Users/_metrics233 UniqueID | cut -d ' ' -f2-)" = 52033
test "$(dscl . -read /Users/_metrics233 PrimaryGroupID | cut -d ' ' -f2-)" = 52033
test "$(dscl . -read /Groups/_metrics233 PrimaryGroupID | cut -d ' ' -f2-)" = 52033
`
	}
	common := verify + `test "$actual" = '` + checksum + `'
chown root "$release/collector.new"; chmod 755 "$release/collector.new"; mv "$release/collector.new" "$release/collector"
` + account + `
had_config=0
if test -f "$base/bootstrap.json"; then cp -p "$base/bootstrap.json" "$base/bootstrap.previous"; had_config=1; fi
cp "$stage/bootstrap.json" "$base/bootstrap.new"; chown "$service_user" "$base/bootstrap.new"; chmod 600 "$base/bootstrap.new"
`
	if platform == "darwin" {
		return prelude + common + `service='/Library/LaunchDaemons/` + label + `.plist'
test ! -L "$service"
had_service=0
if test -f "$service"; then cp -p "$service" "$base/service.previous"; had_service=1; fi
cat > "$base/service.new" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict>
<key>Label</key><string>` + label + `</string><key>ProgramArguments</key><array><string>` + base + `/releases/` + checksum + `/collector</string><string>--config</string><string>` + base + `/bootstrap.json</string></array>
<key>UserName</key><string>_metrics233</string><key>RunAtLoad</key><true/><key>KeepAlive</key><true/><key>ThrottleInterval</key><integer>15</integer><key>HardResourceLimits</key><dict><key>Core</key><integer>0</integer></dict><key>StandardOutPath</key><string>/dev/null</string><key>StandardErrorPath</key><string>/dev/null</string></dict></plist>
PLIST
plutil -lint "$base/service.new" >/dev/null
rollback() {
  launchctl bootout system/` + label + ` >/dev/null 2>&1 || true
  if test "$had_config" = 1; then cp -p "$base/bootstrap.previous" "$base/bootstrap.json"; else rm -f "$base/bootstrap.json"; fi
  if test "$had_service" = 1; then cp -p "$base/service.previous" "$service"; launchctl bootstrap system "$service" >/dev/null 2>&1 || true; else rm -f "$service"; fi
}
trap rollback EXIT
trap 'exit 1' HUP INT TERM
mv "$base/bootstrap.new" "$base/bootstrap.json"
launchctl bootout system/` + label + ` >/dev/null 2>&1 || true
cp "$base/service.new" "$service"; chown root:wheel "$service"; chmod 644 "$service"
launchctl bootstrap system "$service"
launchctl print system/` + label + ` >/dev/null
trap - EXIT HUP INT TERM
echo installed
`
	}
	return prelude + common + `service='/etc/systemd/system/` + label + `.service'
test ! -L "$service"
had_service=0
if test -f "$service"; then cp -p "$service" "$base/service.previous"; had_service=1; fi
cat > "$base/service.new" <<'UNIT'
[Unit]
Description=Metrics233 managed collector
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
User=metrics233-collector
LimitCORE=0
ExecStart=` + base + `/releases/` + checksum + `/collector --config ` + base + `/bootstrap.json
Restart=always
RestartSec=15
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
CapabilityBoundingSet=
UMask=0077
[Install]
WantedBy=multi-user.target
UNIT
rollback() {
  systemctl stop '` + label + `.service' || true
  if test "$had_config" = 1; then cp -p "$base/bootstrap.previous" "$base/bootstrap.json"; else rm -f "$base/bootstrap.json"; fi
  if test "$had_service" = 1; then cp -p "$base/service.previous" "$service"; systemctl daemon-reload; systemctl restart '` + label + `.service' || true; else rm -f "$service"; systemctl daemon-reload; fi
}
trap rollback EXIT
trap 'exit 1' HUP INT TERM
mv "$base/bootstrap.new" "$base/bootstrap.json"
cp "$base/service.new" "$service"; chown root "$service"; chmod 644 "$service"
systemctl daemon-reload
systemctl enable '` + label + `.service'
systemctl restart '` + label + `.service'
systemctl is-active --quiet '` + label + `.service'
trap - EXIT HUP INT TERM
echo installed
`
}
