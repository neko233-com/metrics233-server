package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/neko233-com/metrics233-server/internal/identity"
	"golang.org/x/term"
)

const bootstrapInputLimit = 16 * 1024

type bootstrapAdminInput struct {
	Login    string `json:"login"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func bootstrapAdminCommand(args []string) error {
	flags := flag.NewFlagSet("bootstrap-admin", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("identity-db", "", "absolute protected identity catalog path")
	file := flags.String("file", "", "- reads a bounded JSON object from stdin")
	login := flags.String("login", "", "interactive account login; requires a TTY")
	name := flags.String("name", "", "optional interactive display name")
	email := flags.String("email", "", "optional interactive email")
	if flags.Parse(args) != nil || flags.NArg() != 0 || (*file != "" && *file != "-") || (*login != "" && *file != "") || (*login == "" && (*file != "-" || *name != "" || *email != "")) {
		return errors.New("bootstrap_invalid_arguments")
	}
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return errors.New("bootstrap_requires_linux_root")
	}
	if err := validateBootstrapPath(*path); err != nil {
		return err
	}
	var data []byte
	var err error
	if *login != "" {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return errors.New("bootstrap_interactive_tty_required")
		}
		data, err = interactiveBootstrapInput(*login, *name, *email, func(prompt string) ([]byte, error) {
			fmt.Fprint(os.Stderr, prompt)
			password, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(os.Stderr)
			return password, err
		})
	} else if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintln(os.Stderr, "Enter one bootstrap JSON object (input hidden), then press Enter:")
		data, err = term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
	} else {
		data, err = io.ReadAll(io.LimitReader(os.Stdin, bootstrapInputLimit+1))
	}
	if err != nil || len(data) > bootstrapInputLimit {
		return errors.New("bootstrap_input_rejected")
	}
	defer clear(data)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return bootstrapAdminFromInput(ctx, *path, bytes.NewReader(data), os.Stdout)
}

func interactiveBootstrapInput(login, name, email string, readPassword func(string) ([]byte, error)) ([]byte, error) {
	password, err := readPassword("Password: ")
	if err != nil {
		return nil, errors.New("bootstrap_password_input_failed")
	}
	defer clear(password)
	if len(password) < 8 || len(password) > 1024 {
		return nil, errors.New("bootstrap_password_length_invalid")
	}
	confirmation, err := readPassword("Confirm password: ")
	if err != nil {
		return nil, errors.New("bootstrap_password_input_failed")
	}
	defer clear(confirmation)
	if subtle.ConstantTimeCompare(password, confirmation) != 1 {
		return nil, errors.New("bootstrap_password_confirmation_mismatch")
	}
	return json.Marshal(bootstrapAdminInput{Login: login, Name: name, Email: email, Password: string(password)})
}

// validateBootstrapPath permits only an explicit protected operator-owned
// filesystem boundary. It never creates parents or follows catalog/sidecar links.
func validateBootstrapPath(path string) error {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("bootstrap_absolute_identity_path_required")
	}
	parent := filepath.Dir(path)
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil || resolved != parent {
		return errors.New("bootstrap_protected_parent_required")
	}
	info, err := os.Lstat(parent)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("bootstrap_protected_parent_required")
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		info, err := os.Lstat(path + suffix)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return errors.New("bootstrap_protected_catalog_required")
		}
	}
	return nil
}

// bootstrapAdminFromInput never creates a control store, starts a listener,
// changes an existing account, or includes inputs/database errors in output.
func bootstrapAdminFromInput(ctx context.Context, path string, input io.Reader, output io.Writer) error {
	var value bootstrapAdminInput
	data, err := io.ReadAll(io.LimitReader(input, bootstrapInputLimit+1))
	if err != nil || len(data) > bootstrapInputLimit {
		return errors.New("bootstrap_input_rejected")
	}
	defer clear(data)
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF || value.Login == "" || len(value.Password) < 8 || len(value.Password) > 1024 {
		return errors.New("bootstrap_input_rejected")
	}
	if err = validateBootstrapPath(path); err != nil {
		return err
	}
	if _, err = os.Lstat(path); os.IsNotExist(err) {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if err != nil {
			return errors.New("bootstrap_catalog_unavailable")
		}
		if file.Close() != nil {
			return errors.New("bootstrap_catalog_unavailable")
		}
	}
	catalog, err := identity.Open(path)
	if err != nil {
		return errors.New("bootstrap_catalog_unavailable")
	}
	defer catalog.Close()
	hasUsers, err := catalog.HasUsers(ctx)
	if err != nil {
		return errors.New("bootstrap_catalog_unavailable")
	}
	if hasUsers {
		return errors.New("bootstrap_already_initialized")
	}
	user, err := catalog.BootstrapAdmin(ctx, identity.UserInput{Login: value.Login, Name: value.Name, Email: value.Email, Password: value.Password})
	value.Password = ""
	if errors.Is(err, identity.ErrAlreadyBootstrapped) {
		return errors.New("bootstrap_already_initialized")
	}
	if err != nil {
		return errors.New("bootstrap_administrator_creation_failed")
	}
	return json.NewEncoder(output).Encode(map[string]any{"initialized": true, "user_id": user.ID, "organization_id": 1, "role": "Admin"})
}
