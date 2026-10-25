package store

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/neko233-com/metrics233-server/internal/model"
)

var ErrConnectionConflict = errors.New("connection setup changed or already exists; inspect current resources before retrying")

func (s *Store) initializeConnectionSetups() error {
	_, err := s.DB.Exec(`CREATE TABLE IF NOT EXISTS connection_setups(id TEXT PRIMARY KEY,request_hash TEXT NOT NULL,receipt TEXT NOT NULL);`)
	return err
}

// A keyed fingerprint binds retries to credentials as well as public settings.
// It is never exposed, and does not turn weak passwords into offline hash oracles.
func (s *Store) connectionRequestHash(input model.ConnectionSetupInput) string {
	raw, _ := json.Marshal(input)
	mac := hmac.New(sha256.New, s.secretKey)
	mac.Write([]byte("connection-setup-v1\x00"))
	mac.Write(raw)
	clear(raw)
	return hex.EncodeToString(mac.Sum(nil))
}

// Exposed review proof is separately keyed, so an existing credential rotation
// invalidates an old preview without exposing a password hash to the browser.
func (s *Store) ConnectionPlanProof(input model.ConnectionSetupInput, publicHash string) string {
	mac := hmac.New(sha256.New, s.secretKey)
	mac.Write([]byte("connection-plan-v1\x00" + s.connectionRequestHash(input) + "\x00" + publicHash))
	return hex.EncodeToString(mac.Sum(nil))
}

func readConnectionReceipt(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id, fingerprint string) (model.ConnectionReceipt, error) {
	var receipt model.ConnectionReceipt
	var stored, raw string
	if err := db.QueryRowContext(ctx, `SELECT request_hash,receipt FROM connection_setups WHERE id=?`, id).Scan(&stored, &raw); err != nil {
		return receipt, err
	}
	if !hmac.Equal([]byte(stored), []byte(fingerprint)) {
		return receipt, ErrConnectionConflict
	}
	if json.Unmarshal([]byte(raw), &receipt) != nil {
		return receipt, errors.New("invalid connection receipt")
	}
	var present int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM targets WHERE id=?`, receipt.TargetID).Scan(&present); err != nil {
		return receipt, err
	}
	if present != 1 {
		return receipt, ErrConnectionConflict
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM dashboards WHERE id=?`, receipt.DashboardID).Scan(&present); err != nil {
		return receipt, err
	}
	if present != 1 {
		return receipt, ErrConnectionConflict
	}
	for _, uid := range receipt.RuleUIDs {
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM alert_rules WHERE uid=?`, uid).Scan(&present); err != nil {
			return receipt, err
		}
		if present != 1 {
			return receipt, ErrConnectionConflict
		}
	}
	return receipt, nil
}

// ConnectionRetry returns only a matching completed operation. It never probes
// the endpoint or recreates resources deleted/customized after installation.
func (s *Store) ConnectionRetry(ctx context.Context, input model.ConnectionSetupInput) (model.ConnectionReceipt, error) {
	if err := input.Validate(); err != nil {
		return model.ConnectionReceipt{}, err
	}
	return readConnectionReceipt(ctx, s.DB, input.ID, s.connectionRequestHash(input))
}

// ConnectionTargetSnapshot loads only an existing target's connection contract.
// Scrape timing and failures are intentionally not part of the configuration CAS.
func (s *Store) ConnectionTargetSnapshot(ctx context.Context, id int64) (model.Target, error) {
	return s.connectionTargetSnapshot(ctx, s.DB, id)
}
func (s *Store) connectionTargetSnapshot(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id int64) (model.Target, error) {
	var t model.Target
	var labels string
	var sealed []byte
	err := db.QueryRowContext(ctx, `SELECT name,url,interval_seconds,labels,enabled,COALESCE(c.kind,''),COALESCE(c.username,''),COALESCE(c.database_name,''),COALESCE(c.tls_mode,''),c.secrets FROM targets t LEFT JOIN target_connections c ON c.id=t.id WHERE t.id=?`, id).Scan(&t.Name, &t.URL, &t.IntervalSeconds, &labels, &t.Enabled, &t.Kind, &t.Username, &t.Database, &t.TLSMode, &sealed)
	if err != nil {
		return t, err
	}
	if json.Unmarshal([]byte(labels), &t.Labels) != nil {
		return t, errors.New("invalid target labels")
	}
	if len(sealed) > 0 {
		t.SecureSettings, err = s.openSecrets(fmt.Sprintf("target:%d", id), sealed)
	}
	return t, err
}

func (s *Store) ApplyConnectionSetup(ctx context.Context, input model.ConnectionSetupInput, plan model.ConnectionPlan) (model.ConnectionReceipt, error) {
	var receipt model.ConnectionReceipt
	if err := input.Validate(); err != nil {
		return receipt, err
	}
	if plan.ID != input.ID || plan.Target.ID != 0 || plan.Dashboard.ID != "conn-"+compactID(input.ID) {
		return receipt, ErrConnectionConflict
	}
	if err := plan.Dashboard.Validate(); err != nil {
		return receipt, err
	}
	for i := range plan.Rules {
		plan.Rules[i].Defaults()
		if err := plan.Rules[i].Validate(); err != nil {
			return receipt, err
		}
	}
	ctx = receiverProducerContext(ctx, s)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return receipt, err
	}
	defer tx.Rollback()
	fingerprint := s.connectionRequestHash(input)
	previous, err := readConnectionReceipt(ctx, tx, input.ID, fingerprint)
	if err == nil {
		return previous, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return receipt, err
	}
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM connection_setups`).Scan(&count); err != nil {
		return receipt, err
	}
	if count >= 1024 {
		return receipt, errors.New("at most 1024 quick connections")
	}
	// Never quietly duplicate an existing scrape or overwrite an authored board.
	if input.ExistingTargetID == 0 {
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM targets WHERE name=? AND url=?`, input.Target.Name, input.Target.URL).Scan(&count); err != nil {
			return receipt, err
		}
		if count != 0 {
			return receipt, ErrConnectionConflict
		}
	} else {
		actual, err := s.connectionTargetSnapshot(ctx, tx, input.ExistingTargetID)
		if err != nil {
			return receipt, ErrConnectionConflict
		}
		current := input
		current.Target = actual
		if !hmac.Equal([]byte(fingerprint), []byte(s.connectionRequestHash(current))) {
			return receipt, ErrConnectionConflict
		}
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM connection_setups WHERE json_extract(receipt,'$.target_id')=?`, input.ExistingTargetID).Scan(&count); err != nil {
			return receipt, err
		}
		if count != 0 {
			return receipt, ErrConnectionConflict
		}
	}
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM dashboards WHERE id=?`, plan.Dashboard.ID).Scan(&count); err != nil {
		return receipt, err
	}
	if count != 0 {
		return receipt, ErrConnectionConflict
	}
	target := input.CollectionTarget()
	if input.ExistingTargetID == 0 {
		target, err = s.saveTargetTx(ctx, tx, target)
		if err != nil {
			return receipt, err
		}
	} else {
		target.ID = input.ExistingTargetID
	}
	dashboard, err := saveDashboardTx(ctx, tx, plan.Dashboard, nil)
	if err != nil {
		return receipt, err
	}
	receipt = model.ConnectionReceipt{ID: input.ID, TargetID: target.ID, DashboardID: dashboard.ID, RuleUIDs: []string{}, Profile: plan.Profile, CreatedAt: time.Now().UnixMilli()}
	for _, rule := range plan.Rules {
		saved, err := saveAlertRuleTx(ctx, tx, rule, false)
		if err != nil {
			return receipt, err
		}
		receipt.RuleUIDs = append(receipt.RuleUIDs, saved.UID)
	}
	raw, _ := json.Marshal(receipt)
	if _, err = tx.ExecContext(ctx, `INSERT INTO connection_setups(id,request_hash,receipt) VALUES(?,?,?)`, input.ID, fingerprint, string(raw)); err != nil {
		return receipt, err
	}
	if err = tx.Commit(); err != nil {
		return receipt, err
	}
	return receipt, nil
}

func compactID(id string) string {
	out := make([]byte, 0, len(id))
	for i := range id {
		if id[i] != '-' {
			out = append(out, id[i])
		}
	}
	return string(out)
}

func (s *Store) ConnectionSetups(ctx context.Context) ([]model.ConnectionReceipt, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT receipt FROM connection_setups ORDER BY id LIMIT 1025`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ConnectionReceipt{}
	for rows.Next() {
		var raw string
		var receipt model.ConnectionReceipt
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if json.Unmarshal([]byte(raw), &receipt) != nil {
			return nil, errors.New("invalid connection receipt")
		}
		out = append(out, receipt)
	}
	return out, rows.Err()
}
