package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/neko233-com/metrics233-server/internal/model"
)

const BusinessInstanceLimit = 4096

var ErrBusinessBindingConflict = errors.New("business dashboard binding changed; reload before saving")

func (s *Store) initializeBusinessInstances() error {
	_, err := s.DB.Exec(`CREATE TABLE IF NOT EXISTS business_instances(id TEXT PRIMARY KEY,identity TEXT NOT NULL,reported_ip TEXT NOT NULL DEFAULT '',observed_ip TEXT NOT NULL DEFAULT '',first_seen INTEGER NOT NULL,last_seen INTEGER NOT NULL,binding_version INTEGER NOT NULL DEFAULT 0,binding_mode TEXT NOT NULL DEFAULT 'default',dashboard_id TEXT NOT NULL DEFAULT '');`)
	return err
}

func businessIdentity(labels map[string]string) (model.BusinessIdentity, bool) {
	i := model.BusinessIdentity{Service: labels["service"], Role: labels["role"], ServerID: labels["serverId"], Environment: labels["environment"], Channel: labels["channel"], Instance: labels["instance"], Implementation: labels["implementation"]}
	if i.Service == "" || i.Instance == "" || i.ServerID == "" || (i.Role != "gateway" && i.Role != "logic" && i.Role != "game") {
		return i, false
	}
	for _, value := range i.Labels() {
		if len(value) > 128 || strings.ContainsAny(value, "\x00\r\n") {
			return i, false
		}
	}
	return i, true
}

func BusinessInstanceID(identity model.BusinessIdentity) string {
	raw, _ := json.Marshal(identity)
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

// RecordBusinessInstances only observes already accepted samples. A failed
// auxiliary observation must never cause a successfully committed push to retry.
func (s *Store) RecordBusinessInstances(ctx context.Context, samples []model.Sample, observedIP string) error {
	items := map[string]model.BusinessInstance{}
	for _, sample := range samples {
		identity, ok := businessIdentity(sample.Labels)
		if !ok {
			continue
		}
		id := BusinessInstanceID(identity)
		reported := ""
		if value, err := CanonicalReportingAddress(sample.Labels["ip"]); err == nil {
			reported = value
		}
		items[id] = model.BusinessInstance{ID: id, Identity: identity, ReportedIP: reported}
	}
	if len(items) == 0 {
		return nil
	}
	if observedIP != "" {
		if canonical, err := CanonicalReportingAddress(observedIP); err != nil || canonical != observedIP {
			return errors.New("invalid business observation address")
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statement, err := tx.PrepareContext(ctx, `INSERT INTO business_instances(id,identity,reported_ip,observed_ip,first_seen,last_seen) SELECT ?,?,?,?,?,? WHERE EXISTS(SELECT 1 FROM business_instances WHERE id=?) OR (SELECT count(*) FROM business_instances)<? ON CONFLICT(id) DO UPDATE SET last_seen=MAX(last_seen,excluded.last_seen),reported_ip=excluded.reported_ip,observed_ip=CASE WHEN excluded.observed_ip='' THEN business_instances.observed_ip ELSE excluded.observed_ip END`)
	if err != nil {
		return err
	}
	defer statement.Close()
	now := time.Now().UnixMilli()
	for _, item := range items {
		raw, _ := json.Marshal(item.Identity)
		_, err = statement.ExecContext(ctx, item.ID, string(raw), item.ReportedIP, observedIP, now, now, item.ID, BusinessInstanceLimit)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

const businessColumns = `id,identity,reported_ip,observed_ip,first_seen,last_seen,binding_version,binding_mode,dashboard_id`

func scanBusiness(row interface{ Scan(...any) error }) (model.BusinessInstance, error) {
	var item model.BusinessInstance
	var raw string
	err := row.Scan(&item.ID, &raw, &item.ReportedIP, &item.ObservedIP, &item.FirstSeen, &item.LastSeen, &item.Binding.Version, &item.Binding.Mode, &item.Binding.DashboardID)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &item.Identity)
	}
	return item, err
}
func (s *Store) BusinessInstance(ctx context.Context, id string) (model.BusinessInstance, error) {
	return scanBusiness(s.DB.QueryRowContext(ctx, `SELECT `+businessColumns+` FROM business_instances WHERE id=?`, id))
}

type BusinessInstancePage struct {
	Items      []model.BusinessInstance `json:"items"`
	Next       string                   `json:"next_cursor"`
	Capacity   int                      `json:"capacity"`
	AtCapacity bool                     `json:"at_capacity"`
}

func (s *Store) BusinessInstances(ctx context.Context, after string, limit int) (BusinessInstancePage, error) {
	return s.FindBusinessInstances(ctx, after, "", limit)
}
func (s *Store) FindBusinessInstances(ctx context.Context, after, query string, limit int) (BusinessInstancePage, error) {
	return s.findBusinessInstances(ctx, after, query, "", limit)
}

// FindBusinessInstancesForTemplate pages only live inherit bindings. A copied or
// independently edited dashboard is not an instance of the original template.
// The caller must authorize reading the template before looking up its bindings.
func (s *Store) FindBusinessInstancesForTemplate(ctx context.Context, after, query, templateID string, limit int) (BusinessInstancePage, error) {
	if templateID == "" || len(templateID) > 200 || strings.ContainsAny(templateID, "\x00\r\n") {
		return BusinessInstancePage{}, errors.New("invalid template_id")
	}
	return s.findBusinessInstances(ctx, after, query, templateID, limit)
}

func (s *Store) findBusinessInstances(ctx context.Context, after, query, templateID string, limit int) (BusinessInstancePage, error) {
	page := BusinessInstancePage{Items: []model.BusinessInstance{}, Capacity: BusinessInstanceLimit}
	if limit < 1 || limit > 100 {
		return page, errors.New("limit must be 1–100")
	}
	if len(query) > 128 {
		return page, errors.New("instance search must be at most 128 bytes")
	}
	if after != "" {
		if value, err := hex.DecodeString(after); err != nil || len(value) != 32 {
			return page, errors.New("invalid instance cursor")
		}
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+businessColumns+` FROM business_instances WHERE id>? AND (?='' OR (binding_mode='inherit' AND dashboard_id=?)) AND (?='' OR instr(lower(identity),lower(?))>0 OR instr(reported_ip,?)>0 OR instr(observed_ip,?)>0) ORDER BY id LIMIT ?`, after, templateID, templateID, query, query, query, query, limit+1)
	if err != nil {
		return page, err
	}
	for rows.Next() {
		item, err := scanBusiness(rows)
		if err != nil {
			rows.Close()
			return page, err
		}
		page.Items = append(page.Items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return page, err
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.Next = page.Items[limit-1].ID
	}
	var count int
	err = s.DB.QueryRowContext(ctx, `SELECT count(*) FROM business_instances`).Scan(&count)
	page.AtCapacity = count >= BusinessInstanceLimit
	return page, err
}

// Bindings never grant dashboard permissions and never rewrite the template.
func (s *Store) BindBusinessDashboard(ctx context.Context, id string, binding model.BusinessBinding) (model.BusinessInstance, error) {
	if binding.Version < 0 || (binding.Mode != "default" && binding.Mode != "inherit" && binding.Mode != "independent") || (binding.Mode == "default") != (binding.DashboardID == "") || len(binding.DashboardID) > 200 {
		return model.BusinessInstance{}, errors.New("invalid business dashboard binding")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return model.BusinessInstance{}, err
	}
	defer tx.Rollback()
	if binding.DashboardID != "" {
		var exists int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM dashboards WHERE id=?`, binding.DashboardID).Scan(&exists); err != nil {
			return model.BusinessInstance{}, err
		}
		if exists != 1 {
			return model.BusinessInstance{}, sql.ErrNoRows
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE business_instances SET binding_version=binding_version+1,binding_mode=?,dashboard_id=? WHERE id=? AND binding_version=?`, binding.Mode, binding.DashboardID, id, binding.Version)
	if err != nil {
		return model.BusinessInstance{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return model.BusinessInstance{}, err
	}
	if changed != 1 {
		return model.BusinessInstance{}, ErrBusinessBindingConflict
	}
	item, err := scanBusiness(tx.QueryRowContext(ctx, `SELECT `+businessColumns+` FROM business_instances WHERE id=?`, id))
	if err != nil {
		return item, err
	}
	return item, tx.Commit()
}

func (s *Store) observeBusinessIngest(ctx context.Context, samples []model.Sample) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	if err := s.RecordBusinessInstances(ctx, samples, ""); err != nil {
		slog.Warn("business instance observation could not be recorded")
	}
}
