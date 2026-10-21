package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/neko233-com/metrics233-server/internal/model"
)

func (s *Store) initNotificationCooldown() error {
	_, err := s.DB.Exec(`CREATE TABLE IF NOT EXISTS alert_notification_cooldowns(receiver TEXT NOT NULL,rule_uid TEXT NOT NULL,instance_key TEXT NOT NULL,severity TEXT NOT NULL,notified_at INTEGER NOT NULL,started_at INTEGER NOT NULL,PRIMARY KEY(receiver,rule_uid,instance_key,severity));
CREATE INDEX IF NOT EXISTS alert_notification_cooldown_age ON alert_notification_cooldowns(notified_at);
CREATE TABLE IF NOT EXISTS alert_notification_cooldown_receipts(receiver TEXT NOT NULL,rule_uid TEXT NOT NULL,instance_key TEXT NOT NULL,started_at INTEGER NOT NULL,acknowledged_at INTEGER NOT NULL,PRIMARY KEY(receiver,rule_uid,instance_key,started_at));
CREATE INDEX IF NOT EXISTS alert_notification_cooldown_receipt_age ON alert_notification_cooldown_receipts(acknowledged_at);
CREATE INDEX IF NOT EXISTS alert_notification_cooldown_receipt_period ON alert_notification_cooldown_receipts(rule_uid,instance_key,started_at);
CREATE TABLE IF NOT EXISTS alert_notification_cooldown_waiting(notification_id INTEGER PRIMARY KEY REFERENCES alert_notification_outbox(id) ON DELETE CASCADE,started_at INTEGER NOT NULL);`)
	return err
}

// Every successful firing notification obeys the rule's minimum interval.
// Same-period reminders also retain a longer explicit policy repeat interval.
func notificationCooldownUntilTx(ctx context.Context, tx *sql.Tx, receiver, rule, key, severity string, startedAt, at int64, seconds int, repeatFloor time.Duration) (int64, error) {
	if seconds == 0 {
		return 0, nil
	}
	var notifiedAt, priorStart int64
	err := tx.QueryRowContext(ctx, `SELECT notified_at,started_at FROM alert_notification_cooldowns WHERE receiver=? AND rule_uid=? AND instance_key=? AND severity=?`, receiver, rule, key, severity).Scan(&notifiedAt, &priorStart)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	delay := time.Duration(seconds) * time.Second
	if startedAt == priorStart && repeatFloor > delay {
		delay = repeatFloor
	}
	until := notifiedAt + delay.Milliseconds()
	if until <= at {
		return 0, nil
	}
	return until, nil
}

func currentNotificationCooldownTx(ctx context.Context, tx *sql.Tx, receiver, rule, key, severity string, startedAt, at int64) (int64, error) {
	var seconds int
	err := tx.QueryRowContext(ctx, `SELECT coalesce(json_extract(config,'$.notification_cooldown_seconds'),0) FROM alert_rules WHERE uid=?`, rule).Scan(&seconds)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return notificationCooldownUntilTx(ctx, tx, receiver, rule, key, severity, startedAt, at, seconds, 0)
}

func groupNotificationCooldownTx(ctx context.Context, tx *sql.Tx, receiver string, member groupMember, severity string, at int64, repeat time.Duration) (until int64, skip bool, err error) {
	var seconds int
	err = tx.QueryRowContext(ctx, `SELECT coalesce(json_extract(config,'$.notification_cooldown_seconds'),0) FROM alert_rules WHERE uid=?`, member.Rule).Scan(&seconds)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	// CD must not create reminders when repeat is explicitly disabled. Keep
	// acknowledged members in storage solely for their eventual recovery.
	if seconds > 0 && member.Notified && repeat == 0 {
		return 0, true, nil
	}
	until, err = notificationCooldownUntilTx(ctx, tx, receiver, member.Rule, member.Key, severity, member.StartedAt, at, seconds, repeat)
	return until, false, err
}

func deferNotificationCooldownTx(ctx context.Context, tx *sql.Tx, id, startedAt, until int64) error {
	// Track exact periods even when the first firing has no delay. Retaining
	// this small FK row after ACK lets recovery require that period's receipt.
	if _, err := tx.ExecContext(ctx, `INSERT INTO alert_notification_cooldown_waiting(notification_id,started_at) VALUES(?,?) ON CONFLICT(notification_id) DO NOTHING`, id, startedAt); err != nil {
		return err
	}
	if until == 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx, `UPDATE alert_notification_outbox SET next_attempt_at=?,last_error='Notification cooldown' WHERE id=?`, until, id)
	return err
}

func trackUngroupedFiringCooldownTx(ctx context.Context, tx *sql.Tx, id int64, receiver, rule, key, status string, raw []byte, at int64) error {
	if status != "firing" || rule == "" {
		return nil
	}
	if grouped, _ := ctx.Value(receiverPendingGroupSourceContextKey{}).(bool); grouped {
		return nil
	}
	var seconds int
	err := tx.QueryRowContext(ctx, `SELECT coalesce(json_extract(config,'$.notification_cooldown_seconds'),0) FROM alert_rules WHERE uid=?`, rule).Scan(&seconds)
	if errors.Is(err, sql.ErrNoRows) || err == nil && seconds == 0 {
		return nil
	}
	if err != nil {
		return err
	}
	var payload struct {
		CommonLabels map[string]string `json:"commonLabels"`
		Alerts       []struct {
			StartsAt string `json:"startsAt"`
		} `json:"alerts"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	if len(payload.Alerts) == 0 {
		return nil
	}
	started, err := time.Parse(time.RFC3339Nano, payload.Alerts[0].StartsAt)
	if err != nil {
		return err
	}
	until, err := notificationCooldownUntilTx(ctx, tx, receiver, rule, key, payload.CommonLabels["severity"], started.UnixMilli(), at, seconds, 0)
	if err != nil {
		return err
	}
	return deferNotificationCooldownTx(ctx, tx, id, started.UnixMilli(), until)
}

// Recheck immediately before sending as well as during claiming. A deferred
// period must still be active; cancellation does not consume a retry attempt.
func validCooldownNotificationTx(ctx context.Context, tx *sql.Tx, id, at int64) (bool, error) {
	var startedAt int64
	err := tx.QueryRowContext(ctx, `SELECT started_at FROM alert_notification_cooldown_waiting WHERE notification_id=?`, id).Scan(&startedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	var receiver, rule, key, raw, state, status string
	var attempts int
	if err := tx.QueryRowContext(ctx, `SELECT contact_point_uid,rule_uid,instance_key,payload,state,attempts,status FROM alert_notification_outbox WHERE id=?`, id).Scan(&receiver, &rule, &key, &raw, &state, &attempts, &status); err != nil {
		return false, err
	}
	if status == "resolved" {
		var acknowledged int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM alert_notification_cooldown_receipts WHERE receiver=? AND rule_uid=? AND instance_key=? AND started_at=?`, receiver, rule, key, startedAt).Scan(&acknowledged); err != nil {
			return false, err
		}
		if acknowledged > 0 {
			return true, nil
		}
		_, err := tx.ExecContext(ctx, `UPDATE alert_notification_outbox SET state='canceled',last_error='Firing was not acknowledged for this cooldown period',next_attempt_at=0 WHERE id=?`, id)
		return false, err
	}
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM alert_notification_active a JOIN alert_rules r ON r.uid=a.rule_uid WHERE a.rule_uid=? AND a.instance_key=? AND a.started_at=? AND NOT coalesce(json_extract(r.config,'$.paused'),0)`, rule, key, startedAt).Scan(&active); err != nil {
		return false, err
	}
	if active == 0 {
		_, err := tx.ExecContext(ctx, `UPDATE alert_notification_outbox SET state='canceled',last_error='Resolved during notification cooldown',next_attempt_at=0 WHERE id=?`, id)
		return false, err
	}
	// Attempts already made keep their original backoff and body. Only an
	// unsent first attempt can be moved by a newer successful ACK.
	if attempts > 0 && (state != "sending" || attempts > 1) {
		return true, nil
	}
	labels, err := notificationLabels(raw)
	if err != nil {
		return false, err
	}
	until, err := currentNotificationCooldownTx(ctx, tx, receiver, rule, key, labels["severity"], startedAt, at)
	if err != nil || until == 0 {
		return err == nil, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE alert_notification_outbox SET state='pending',next_attempt_at=?,last_error='Notification cooldown',attempts=attempts-CASE WHEN state='sending' THEN 1 ELSE 0 END WHERE id=?`, until, id)
	return false, err
}

func cooldownResolvedReceiversTx(ctx context.Context, tx *sql.Tx, rule, key string, startedAt int64) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT n.contact_point_uid FROM alert_notification_outbox n JOIN alert_notification_cooldown_waiting w ON w.notification_id=n.id WHERE n.rule_uid=? AND n.instance_key=? AND w.started_at=? AND n.state IN ('pending','suppressed')`, rule, key, startedAt)
	if err != nil {
		return nil, err
	}
	deferred := map[string]bool{}
	for rows.Next() {
		var receiver string
		if err := rows.Scan(&receiver); err != nil {
			rows.Close()
			return nil, err
		}
		deferred[receiver] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE alert_notification_outbox SET state='canceled',last_error='Resolved during notification cooldown',next_attempt_at=0 WHERE id IN (SELECT n.id FROM alert_notification_outbox n JOIN alert_notification_cooldown_waiting w ON w.notification_id=n.id WHERE n.rule_uid=? AND n.instance_key=? AND w.started_at=? AND n.state IN ('pending','suppressed'))`, rule, key, startedAt); err != nil {
		return nil, err
	}
	for receiver := range deferred {
		var possibleDelivery int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM alert_notification_outbox WHERE contact_point_uid=? AND rule_uid=? AND instance_key=? AND created_at>=? AND status='firing' AND state IN ('sent','sending')`, receiver, rule, key, startedAt).Scan(&possibleDelivery); err != nil {
			return nil, err
		}
		if possibleDelivery > 0 {
			delete(deferred, receiver)
		}
	}
	return deferred, nil
}

func pruneResolvedCooldownReceiptsTx(ctx context.Context, tx *sql.Tx, rule, key string, startedAt int64) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM alert_notification_cooldown_receipts AS c WHERE rule_uid=? AND instance_key=? AND started_at=? AND NOT EXISTS(SELECT 1 FROM alert_notification_outbox n JOIN alert_notification_cooldown_waiting w ON w.notification_id=n.id WHERE n.contact_point_uid=c.receiver AND n.rule_uid=c.rule_uid AND n.instance_key=c.instance_key AND n.status='resolved' AND n.state IN ('pending','sending','suppressed') AND w.started_at=c.started_at) AND NOT EXISTS(SELECT 1 FROM alert_notification_group_members m JOIN alert_notification_groups g ON g.group_key=m.group_key WHERE g.receiver=c.receiver AND m.rule_uid=c.rule_uid AND m.instance_key=c.instance_key AND m.started_at=c.started_at AND m.status='resolved')`, rule, key, startedAt)
	return err
}

// Persist the ACK and its cooldown anchor in the same transaction. Editing or
// pausing a rule resets evaluation runtime but never resets these anchors.
func recordNotificationCooldownTx(ctx context.Context, tx *sql.Tx, delivery model.NotificationDelivery, at int64) error {
	if delivery.Status == "test" {
		return nil
	}
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT payload FROM alert_notification_outbox WHERE id=?`, delivery.ID).Scan(&raw); err != nil {
		return err
	}
	var payload struct {
		CommonLabels map[string]string `json:"commonLabels"`
		Alerts       []struct {
			Fingerprint string            `json:"fingerprint"`
			StartsAt    string            `json:"startsAt"`
			Labels      map[string]string `json:"labels"`
		} `json:"alerts"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return err
	}
	var members []groupMember
	var groupRaw string
	err := tx.QueryRowContext(ctx, `SELECT members FROM alert_notification_group_deliveries WHERE notification_id=?`, delivery.ID).Scan(&groupRaw)
	if err == nil {
		if err := json.Unmarshal([]byte(groupRaw), &members); err != nil {
			return err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	} else if (delivery.Status == "firing" || delivery.Status == "resolved") && len(payload.Alerts) > 0 {
		started, err := time.Parse(time.RFC3339Nano, payload.Alerts[0].StartsAt)
		if err != nil {
			return err
		}
		members = []groupMember{{Rule: delivery.RuleUID, Key: delivery.InstanceKey, StartedAt: started.UnixMilli(), Status: delivery.Status}}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM alert_notification_cooldowns WHERE notified_at<?`, at-int64(7*24*time.Hour/time.Millisecond)); err != nil {
		return err
	}
	// Successful firing receipts survive outbox/history retention while their
	// period is active or a recovery is still queued. Otherwise aged inactive
	// metadata can be reclaimed independently of the cooldown duration.
	if _, err := tx.ExecContext(ctx, `DELETE FROM alert_notification_cooldown_receipts AS c WHERE acknowledged_at<? AND NOT EXISTS(SELECT 1 FROM alert_notification_active a WHERE a.rule_uid=c.rule_uid AND a.instance_key=c.instance_key AND a.started_at=c.started_at) AND NOT EXISTS(SELECT 1 FROM alert_notification_outbox n JOIN alert_notification_cooldown_waiting w ON w.notification_id=n.id WHERE n.contact_point_uid=c.receiver AND n.rule_uid=c.rule_uid AND n.instance_key=c.instance_key AND n.status='resolved' AND n.state IN ('pending','sending','suppressed') AND w.started_at=c.started_at) AND NOT EXISTS(SELECT 1 FROM alert_notification_group_members m JOIN alert_notification_groups g ON g.group_key=m.group_key WHERE g.receiver=c.receiver AND m.rule_uid=c.rule_uid AND m.instance_key=c.instance_key AND m.started_at=c.started_at AND m.status='resolved')`, at-int64(7*24*time.Hour/time.Millisecond)); err != nil {
		return err
	}
	for _, member := range members {
		if member.Status == "resolved" {
			if _, err := tx.ExecContext(ctx, `DELETE FROM alert_notification_cooldown_receipts WHERE receiver=? AND rule_uid=? AND instance_key=? AND started_at=?`, delivery.ContactPointUID, member.Rule, member.Key, member.StartedAt); err != nil {
				return err
			}
			continue
		}
		if member.Status != "firing" {
			continue
		}
		severity := payload.CommonLabels["severity"]
		for _, alert := range payload.Alerts {
			if alert.Fingerprint == member.Key {
				severity = alert.Labels["severity"]
				break
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO alert_notification_cooldowns(receiver,rule_uid,instance_key,severity,notified_at,started_at) VALUES(?,?,?,?,?,?) ON CONFLICT(receiver,rule_uid,instance_key,severity) DO UPDATE SET notified_at=excluded.notified_at,started_at=excluded.started_at WHERE excluded.notified_at>=notified_at`, delivery.ContactPointUID, member.Rule, member.Key, severity, at, member.StartedAt); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO alert_notification_cooldown_receipts(receiver,rule_uid,instance_key,started_at,acknowledged_at) VALUES(?,?,?,?,?) ON CONFLICT(receiver,rule_uid,instance_key,started_at) DO UPDATE SET acknowledged_at=max(acknowledged_at,excluded.acknowledged_at)`, delivery.ContactPointUID, member.Rule, member.Key, member.StartedAt, at); err != nil {
			return err
		}
	}
	return nil
}
