package store

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/telemetry"
)

var ErrHostConflict = errors.New("host changed; read it again before saving")

func (s *Store) initializeManagedHosts() error {
	_, err := s.DB.Exec(`CREATE TABLE IF NOT EXISTS managed_hosts(id TEXT PRIMARY KEY,version INTEGER NOT NULL,config TEXT NOT NULL,secrets BLOB NOT NULL,last_seen INTEGER NOT NULL DEFAULT 0,heartbeat TEXT NOT NULL DEFAULT '{}');
CREATE TABLE IF NOT EXISTS managed_host_events(id INTEGER PRIMARY KEY AUTOINCREMENT,host_id TEXT NOT NULL,at INTEGER NOT NULL,actor TEXT NOT NULL,action TEXT NOT NULL,result TEXT NOT NULL);`)
	return err
}

func (s *Store) ManagedHosts(ctx context.Context) ([]model.ManagedHost, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,version,config,secrets,last_seen,heartbeat FROM managed_hosts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ManagedHost{}
	for rows.Next() {
		h, _, err := s.readManagedHost(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) readManagedHost(scan func(...any) error) (model.ManagedHost, map[string]string, error) {
	var h model.ManagedHost
	var id, raw, heartbeat string
	var version, last int64
	var sealed []byte
	if err := scan(&id, &version, &raw, &sealed, &last, &heartbeat); err != nil {
		return h, nil, err
	}
	if err := json.Unmarshal([]byte(raw), &h); err != nil {
		return h, nil, err
	}
	secrets, err := s.openSecrets("managed-host:"+id, sealed)
	if err != nil {
		return h, nil, err
	}
	h.ID = id
	h.Version = version
	h.LastSeen = last
	h.SecureSettings = nil
	h.Target.SecureSettings = nil
	h.SecureFields = map[string]bool{}
	for k, v := range secrets {
		if k != "agent_token" && v != "" {
			h.SecureFields[k] = true
		}
	}
	var report model.HostHeartbeat
	if err := json.Unmarshal([]byte(heartbeat), &report); err != nil {
		return h, nil, err
	}
	h.AgentVersion = report.Version
	h.AgentState = report.State
	h.AgentOS = report.OS
	h.AgentArch = report.Arch
	h.AgentSHA = report.SHA
	h.AppliedVersion = report.AppliedVersion
	// Only a count is retained in the heartbeat catalog; sample data stays in TSDB.
	var count struct {
		Count     int                  `json:"count"`
		Resources *model.HostResources `json:"resources"`
	}
	_ = json.Unmarshal([]byte(heartbeat), &count)
	h.Samples = count.Count
	h.Resources = count.Resources
	return h, secrets, nil
}

func (s *Store) ManagedHost(ctx context.Context, id string) (model.ManagedHost, map[string]string, error) {
	return s.readManagedHost(s.DB.QueryRowContext(ctx, `SELECT id,version,config,secrets,last_seen,heartbeat FROM managed_hosts WHERE id=?`, id).Scan)
}

func (s *Store) SaveManagedHost(ctx context.Context, h model.ManagedHost, actor string) (model.ManagedHost, error) {
	return s.saveManagedHost(ctx, h, actor, false)
}

func (s *Store) RevokeManagedHost(ctx context.Context, h model.ManagedHost, actor string) (model.ManagedHost, error) {
	h.Enabled = false
	h.Revoked = true
	return s.saveManagedHost(ctx, h, actor, true)
}

func (s *Store) saveManagedHost(ctx context.Context, h model.ManagedHost, actor string, rotate bool) (model.ManagedHost, error) {
	if err := h.Validate(); err != nil {
		return h, err
	}
	if h.ID != "" {
		if _, err := uuid.Parse(h.ID); err != nil {
			return h, errors.New("invalid host identity")
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return h, err
	}
	defer tx.Rollback()
	secrets := map[string]string{}
	if h.ID == "" {
		if h.Version != 0 {
			return h, ErrHostConflict
		}
		h.ID = uuid.NewString()
		secret := make([]byte, 32)
		if _, err = rand.Read(secret); err != nil {
			return h, err
		}
		secrets["agent_token"] = base64.RawURLEncoding.EncodeToString(secret)
	} else {
		var current int64
		var sealed []byte
		if err = tx.QueryRowContext(ctx, `SELECT version,secrets FROM managed_hosts WHERE id=?`, h.ID).Scan(&current, &sealed); err != nil {
			return h, err
		}
		if current != h.Version {
			return h, ErrHostConflict
		}
		secrets, err = s.openSecrets("managed-host:"+h.ID, sealed)
		if err != nil {
			return h, err
		}
	}
	for k, v := range h.SecureSettings {
		if v == "" {
			delete(secrets, k)
		} else {
			secrets[k] = v
		}
	}
	if rotate {
		secret := make([]byte, 32)
		if _, err = rand.Read(secret); err != nil {
			return h, err
		}
		secrets["agent_token"] = base64.RawURLEncoding.EncodeToString(secret)
	} else if h.Enabled {
		h.Revoked = false
	}
	h.Version++
	h.SecureSettings = nil
	h.SecureFields = nil
	h.LastSeen = 0
	h.AgentVersion = ""
	h.AgentState = ""
	h.AgentOS = ""
	h.AgentArch = ""
	h.AgentSHA = ""
	h.Samples = 0
	h.Resources = nil
	h.AppliedVersion = 0
	h.Target.Enabled = true
	h.Target.LastScrape = 0
	h.Target.LastError = ""
	h.Target.Samples = 0
	h.Target.DurationMS = 0
	h.Target.SecureFields = nil
	raw, err := json.Marshal(h)
	if err != nil {
		return h, err
	}
	sealed, err := s.seal("managed-host:"+h.ID, secrets)
	if err != nil {
		return h, err
	}
	if h.Version == 1 {
		_, err = tx.ExecContext(ctx, `INSERT INTO managed_hosts(id,version,config,secrets) VALUES(?,?,?,?)`, h.ID, h.Version, string(raw), sealed)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE managed_hosts SET version=?,config=?,secrets=? WHERE id=?`, h.Version, string(raw), sealed, h.ID)
	}
	if err != nil {
		return h, err
	}
	action, result := "configure", "saved"
	if rotate {
		action, result = "revoke", "credential_rotated_and_paused"
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO managed_host_events(host_id,at,actor,action,result) VALUES(?,?,?,?,?)`, h.ID, time.Now().UnixMilli(), actor, action, result)
	if err != nil {
		return h, err
	}
	if err = tx.Commit(); err != nil {
		return h, err
	}
	h, _, err = s.ManagedHost(ctx, h.ID)
	return h, err
}

func (s *Store) AuthenticateHost(ctx context.Context, id, token string) (model.ManagedHost, map[string]string, error) {
	h, secrets, err := s.ManagedHost(ctx, id)
	if err != nil || len(token) != 43 || subtle.ConstantTimeCompare([]byte(token), []byte(secrets["agent_token"])) != 1 {
		return model.ManagedHost{}, nil, errors.New("invalid host credential")
	}
	return h, secrets, nil
}

func (s *Store) RecordHostHeartbeat(ctx context.Context, id string, report model.HostHeartbeat, count int) error {
	now := time.Now().UnixMilli()
	resources := telemetry.HostResourceSnapshot(report.Samples, now)
	// Preserve the timestamp of the last real observation on paused/transport-only
	// heartbeats. Clients must check freshness and enabled state before coloring it.
	if resources == nil {
		var previous string
		if err := s.DB.QueryRowContext(ctx, `SELECT heartbeat FROM managed_hosts WHERE id=?`, id).Scan(&previous); err != nil {
			return err
		}
		var old struct {
			Resources *model.HostResources `json:"resources"`
		}
		if json.Unmarshal([]byte(previous), &old) == nil {
			resources = old.Resources
		}
	}
	report.Samples = nil
	raw, err := json.Marshal(struct {
		model.HostHeartbeat
		Count     int                  `json:"count"`
		Resources *model.HostResources `json:"resources,omitempty"`
	}{report, count, resources})
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE managed_hosts SET last_seen=?,heartbeat=? WHERE id=?`, now, string(raw), id)
	return err
}

func (s *Store) HostEvent(ctx context.Context, id, actor, action, result string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO managed_host_events(host_id,at,actor,action,result) VALUES(?,?,?,?,?)`, id, time.Now().UnixMilli(), actor, action, result)
	return err
}

func (s *Store) HostEvents(ctx context.Context, id string) ([]map[string]any, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT at,actor,action,result FROM managed_host_events WHERE host_id=? ORDER BY id DESC LIMIT 100`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var at int64
		var actor, action, result string
		if err := rows.Scan(&at, &actor, &action, &result); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"at": at, "actor": actor, "action": action, "result": result})
	}
	return out, rows.Err()
}
