package alerting

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/stretchr/testify/require"
)

type cooldownFixture struct {
	e        *Engine
	path     string
	at       time.Time
	received chan receivedGroup
	failNext atomic.Bool
}

func newCooldownFixture(t *testing.T, grouped bool) *cooldownFixture {
	t.Helper()
	f := &cooldownFixture{path: filepath.Join(t.TempDir(), "cooldown.db"), at: time.Now().Add(time.Hour).Truncate(time.Second), received: make(chan receivedGroup, 32)}
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload receivedGroup
		if json.NewDecoder(r.Body).Decode(&payload) != nil {
			t.Error("invalid notification payload")
		}
		f.received <- payload
		if f.failNext.Swap(false) {
			w.WriteHeader(503)
		} else {
			w.WriteHeader(204)
		}
	}))
	t.Cleanup(webhook.Close)
	db, err := store.Open(f.path)
	require.NoError(t, err)
	if grouped {
		f.e = groupEngine(t, db, webhook.URL, "0s", "1s", "0s")
	} else {
		addNotificationPoint(t, db, webhook.URL)
		_, err = db.SaveAlertRule(t.Context(), model.AlertRule{UID: "service", Title: "Service", Expr: "vector(1)", Condition: "nonzero"})
		require.NoError(t, err)
		f.e = &Engine{Store: db, active: map[string]bool{}, slots: make(chan struct{}, 4)}
	}
	rule, err := db.AlertRule(t.Context(), "service")
	require.NoError(t, err)
	rule.NotificationCooldownSeconds = 3600
	_, err = db.SaveAlertRule(t.Context(), rule.AlertRule)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.e.Store.DB.Close()) })
	return f
}

func (f *cooldownFixture) restart(t *testing.T) {
	t.Helper()
	require.NoError(t, f.e.Store.DB.Close())
	db, err := store.Open(f.path)
	require.NoError(t, err)
	f.e = &Engine{Store: db, active: map[string]bool{}, slots: make(chan struct{}, 4)}
}

func TestNotificationCooldownAcrossRecoveryAndRestart(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		t.Run(strconv.FormatBool(grouped), func(t *testing.T) {
			f := newCooldownFixture(t, grouped)
			evaluateGroup(t, f.e, f.at, map[string]float64{"a": 1})
			processGroup(t, f.e, f.at, true)
			require.Equal(t, "firing", (<-f.received).Status)
			evaluateGroup(t, f.e, f.at.Add(10*time.Minute), map[string]float64{"a": 9})
			processGroup(t, f.e, f.at.Add(10*time.Minute), false)
			evaluateGroup(t, f.e, f.at.Add(20*time.Minute), map[string]float64{"a": 0})
			processGroup(t, f.e, f.at.Add(20*time.Minute), true)
			require.Equal(t, "resolved", (<-f.received).Status)
			evaluateGroup(t, f.e, f.at.Add(30*time.Minute), map[string]float64{"a": 1})
			processGroup(t, f.e, f.at.Add(30*time.Minute), false)
			f.restart(t)
			processGroup(t, f.e, f.at.Add(59*time.Minute), false)
			processGroup(t, f.e, f.at.Add(time.Hour), true)
			require.Equal(t, "firing", (<-f.received).Status)
			processGroup(t, f.e, f.at.Add(2*time.Hour), false)
		})
	}
}

func TestNotificationCooldownCancelsUndeliveredPeriodsWithoutOrphanRecovery(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		t.Run(strconv.FormatBool(grouped), func(t *testing.T) {
			f := newCooldownFixture(t, grouped)
			// A first firing that resolves before the worker starts is never sent.
			evaluateGroup(t, f.e, f.at, map[string]float64{"a": 1})
			evaluateGroup(t, f.e, f.at.Add(time.Second), map[string]float64{"a": 0})
			processGroup(t, f.e, f.at.Add(2*time.Second), false)
			require.Empty(t, f.received)
			// No successful ACK means this next firing must be immediate.
			evaluateGroup(t, f.e, f.at.Add(3*time.Second), map[string]float64{"a": 1})
			processGroup(t, f.e, f.at.Add(3*time.Second), true)
			<-f.received
			evaluateGroup(t, f.e, f.at.Add(5*time.Second), map[string]float64{"a": 0})
			processGroup(t, f.e, f.at.Add(5*time.Second), true)
			<-f.received
			evaluateGroup(t, f.e, f.at.Add(10*time.Second), map[string]float64{"a": 1})
			processGroup(t, f.e, f.at.Add(10*time.Second), false)
			evaluateGroup(t, f.e, f.at.Add(15*time.Second), map[string]float64{"a": 0})
			processGroup(t, f.e, f.at.Add(15*time.Second), false)
			f.restart(t)
			processGroup(t, f.e, f.at.Add(2*time.Hour), false)
			require.Empty(t, f.received)
		})
	}
}

func TestNotificationCooldownNeverConsumesFailedDeliveryRetry(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		t.Run(strconv.FormatBool(grouped), func(t *testing.T) {
			f := newCooldownFixture(t, grouped)
			f.failNext.Store(true)
			evaluateGroup(t, f.e, f.at, map[string]float64{"a": 1})
			processGroup(t, f.e, f.at, true)
			first := <-f.received
			var count int
			require.NoError(t, f.e.Store.DB.QueryRow(`SELECT count(*) FROM alert_notification_cooldowns`).Scan(&count))
			require.Zero(t, count)
			processGroup(t, f.e, f.at.Add(4*time.Second), false)
			processGroup(t, f.e, f.at.Add(5*time.Second), true)
			second := <-f.received
			require.Equal(t, first.ID, second.ID)
			history, err := f.e.Store.Notifications(t.Context(), "webhook", 10)
			require.NoError(t, err)
			require.Equal(t, 2, history[0].Attempts)
			require.Equal(t, "sent", history[0].State)
			require.NoError(t, f.e.Store.DB.QueryRow(`SELECT count(*) FROM alert_notification_cooldowns`).Scan(&count))
			require.Equal(t, 1, count)
		})
	}
}

func TestNotificationCooldownFailedFirstPeriodRecoveryIsCanceled(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		t.Run(strconv.FormatBool(grouped), func(t *testing.T) {
			f := newCooldownFixture(t, grouped)
			f.failNext.Store(true)
			evaluateGroup(t, f.e, f.at, map[string]float64{"a": 1})
			processGroup(t, f.e, f.at, true)
			<-f.received
			evaluateGroup(t, f.e, f.at.Add(time.Second), map[string]float64{"a": 0})
			processGroup(t, f.e, f.at.Add(10*time.Second), false)
			require.Empty(t, f.received)
		})
	}
}

func TestNotificationCooldownNewInstanceAndSeverityRemainImmediate(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		t.Run(strconv.FormatBool(grouped), func(t *testing.T) {
			f := newCooldownFixture(t, grouped)
			evaluateGroup(t, f.e, f.at, map[string]float64{"a": 1})
			processGroup(t, f.e, f.at, true)
			<-f.received
			evaluateGroup(t, f.e, f.at.Add(5*time.Second), map[string]float64{"a": 1, "b": 1})
			processGroup(t, f.e, f.at.Add(5*time.Second), true)
			newInstance := <-f.received
			found := false
			for _, alert := range newInstance.Alerts {
				if alert.Labels["instance"] == "b" {
					found = true
				}
			}
			require.True(t, found)
			// Severity is part of effective instance identity. A warning's ACK
			// cannot suppress a new critical identity, even on the same resource.
			f.e.Query = func(context.Context, string, time.Time) ([]Value, error) {
				return []Value{{Value: 1, Labels: map[string]string{"team": "ops", "instance": "a", "severity": "critical"}}}, nil
			}
			_, err := f.e.Evaluate(t.Context(), "service", f.at.Add(10*time.Second))
			require.NoError(t, err)
			processGroup(t, f.e, f.at.Add(10*time.Second), true)
			critical := <-f.received
			found = false
			for _, alert := range critical.Alerts {
				if alert.Labels["severity"] == "critical" {
					found = true
				}
			}
			require.True(t, found)
		})
	}
}

func TestNotificationCooldownRulePauseEditAndContactDisableCannotResetACK(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		t.Run(strconv.FormatBool(grouped), func(t *testing.T) {
			f := newCooldownFixture(t, grouped)
			evaluateGroup(t, f.e, f.at, map[string]float64{"a": 1})
			processGroup(t, f.e, f.at, true)
			<-f.received
			rule, err := f.e.Store.AlertRule(t.Context(), "service")
			require.NoError(t, err)
			rule.Paused = true
			rule, err = f.e.Store.SaveAlertRule(t.Context(), rule.AlertRule)
			require.NoError(t, err)
			rule.Paused = false
			_, err = f.e.Store.SaveAlertRule(t.Context(), rule.AlertRule)
			require.NoError(t, err)
			point, err := f.e.Store.ContactPoint(t.Context(), "webhook")
			require.NoError(t, err)
			point.Disabled = true
			point, err = f.e.Store.SaveContactPoint(t.Context(), model.ContactPointInput{ContactPoint: point})
			require.NoError(t, err)
			point.Disabled = false
			_, err = f.e.Store.SaveContactPoint(t.Context(), model.ContactPointInput{ContactPoint: point})
			require.NoError(t, err)
			var anchor int64
			require.NoError(t, f.e.Store.DB.QueryRow(`SELECT notified_at FROM alert_notification_cooldowns`).Scan(&anchor))
			require.Equal(t, f.at.UnixMilli(), anchor)
			// Allow the already-ACKed period's recovery, then retrigger the same
			// identity. Saving a rule reset runtime but must not reset the CD.
			for range 4 {
				processed, err := f.e.ProcessNotification(t.Context(), f.at.Add(2*time.Second))
				require.NoError(t, err)
				if !processed {
					break
				}
			}
			for len(f.received) > 0 {
				require.Equal(t, "resolved", (<-f.received).Status)
			}
			evaluateGroup(t, f.e, f.at.Add(10*time.Second), map[string]float64{"a": 1})
			processGroup(t, f.e, f.at.Add(10*time.Second), false)
			f.restart(t)
			processGroup(t, f.e, f.at.Add(time.Hour), true)
			require.Equal(t, "firing", (<-f.received).Status)
		})
	}
}

func TestNotificationCooldownIsMinimumForExplicitSamePeriodRepeat(t *testing.T) {
	f := newCooldownFixture(t, true)
	rule, err := f.e.Store.AlertRule(t.Context(), "service")
	require.NoError(t, err)
	rule.NotificationCooldownSeconds = 300
	_, err = f.e.Store.SaveAlertRule(t.Context(), rule.AlertRule)
	require.NoError(t, err)
	policy, err := f.e.Store.NotificationPolicy(t.Context())
	require.NoError(t, err)
	policy.RepeatInterval = "10s"
	_, err = f.e.Store.SaveNotificationPolicy(t.Context(), policy)
	require.NoError(t, err)
	evaluateGroup(t, f.e, f.at, map[string]float64{"a": 1})
	processGroup(t, f.e, f.at, true)
	<-f.received
	processGroup(t, f.e, f.at.Add(9*time.Second), false)
	processGroup(t, f.e, f.at.Add(10*time.Second), false)
	processGroup(t, f.e, f.at.Add(299*time.Second), false)
	processGroup(t, f.e, f.at.Add(300*time.Second), true)
	require.Equal(t, "firing", (<-f.received).Status)
}

func TestNotificationCooldownZeroRetainsLegacyRepeat(t *testing.T) {
	f := newCooldownFixture(t, true)
	rule, err := f.e.Store.AlertRule(t.Context(), "service")
	require.NoError(t, err)
	rule.NotificationCooldownSeconds = 0
	_, err = f.e.Store.SaveAlertRule(t.Context(), rule.AlertRule)
	require.NoError(t, err)
	policy, err := f.e.Store.NotificationPolicy(t.Context())
	require.NoError(t, err)
	policy.RepeatInterval = "10s"
	_, err = f.e.Store.SaveNotificationPolicy(t.Context(), policy)
	require.NoError(t, err)
	evaluateGroup(t, f.e, f.at, map[string]float64{"a": 1})
	processGroup(t, f.e, f.at, true)
	<-f.received
	processGroup(t, f.e, f.at.Add(9*time.Second), false)
	processGroup(t, f.e, f.at.Add(10*time.Second), true)
	require.Equal(t, "firing", (<-f.received).Status)
}

func TestNotificationCooldownNewMembersNeverCreateDisabledOrEarlyRepeats(t *testing.T) {
	for _, repeat := range []string{"0s", "4h"} {
		t.Run(repeat, func(t *testing.T) {
			f := newCooldownFixture(t, true)
			policy, err := f.e.Store.NotificationPolicy(t.Context())
			require.NoError(t, err)
			policy.RepeatInterval = repeat
			_, err = f.e.Store.SaveNotificationPolicy(t.Context(), policy)
			require.NoError(t, err)
			evaluateGroup(t, f.e, f.at, map[string]float64{"a": 1})
			processGroup(t, f.e, f.at, true)
			<-f.received
			evaluateGroup(t, f.e, f.at.Add(5*time.Second), map[string]float64{"a": 1, "b": 1})
			processGroup(t, f.e, f.at.Add(5*time.Second), true)
			newMember := <-f.received
			require.Len(t, newMember.Alerts, 1)
			require.Equal(t, "b", newMember.Alerts[0].Labels["instance"])
			processGroup(t, f.e, f.at.Add(time.Hour), false)
			if repeat == "0s" {
				processGroup(t, f.e, f.at.Add(8*time.Hour), false)
			} else {
				processGroup(t, f.e, f.at.Add(4*time.Hour), true)
				require.Equal(t, "a", (<-f.received).Alerts[0].Labels["instance"])
			}
		})
	}
}

func TestNotificationCooldownFailedReminderKeepsOriginalRetry(t *testing.T) {
	f := newCooldownFixture(t, true)
	rule, err := f.e.Store.AlertRule(t.Context(), "service")
	require.NoError(t, err)
	rule.NotificationCooldownSeconds = 300
	_, err = f.e.Store.SaveAlertRule(t.Context(), rule.AlertRule)
	require.NoError(t, err)
	policy, err := f.e.Store.NotificationPolicy(t.Context())
	require.NoError(t, err)
	policy.RepeatInterval = "10s"
	_, err = f.e.Store.SaveNotificationPolicy(t.Context(), policy)
	require.NoError(t, err)
	evaluateGroup(t, f.e, f.at, map[string]float64{"a": 1})
	processGroup(t, f.e, f.at, true)
	<-f.received
	f.failNext.Store(true)
	processGroup(t, f.e, f.at.Add(300*time.Second), true)
	failed := <-f.received
	processGroup(t, f.e, f.at.Add(304*time.Second), false)
	processGroup(t, f.e, f.at.Add(305*time.Second), true)
	require.Equal(t, failed.ID, (<-f.received).ID)
	processGroup(t, f.e, f.at.Add(600*time.Second), false)
	processGroup(t, f.e, f.at.Add(605*time.Second), true)
	<-f.received
}

func TestNotificationCooldownSilenceResumeStillWaitsAndKeepsRecoveryReceipt(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		for _, recoverEarly := range []bool{false, true} {
			t.Run(strconv.FormatBool(grouped)+"/"+strconv.FormatBool(recoverEarly), func(t *testing.T) {
				f := newCooldownFixture(t, grouped)
				evaluateGroup(t, f.e, f.at, map[string]float64{"a": 1})
				processGroup(t, f.e, f.at, true)
				<-f.received
				addSilence(t, f.e.Store, f.at.Add(10*time.Second), f.at.Add(20*time.Second))
				processGroup(t, f.e, f.at.Add(10*time.Second), false)
				processGroup(t, f.e, f.at.Add(20*time.Second), false)
				if recoverEarly {
					evaluateGroup(t, f.e, f.at.Add(30*time.Second), map[string]float64{"a": 0})
					processGroup(t, f.e, f.at.Add(30*time.Second), true)
					require.Equal(t, "resolved", (<-f.received).Status)
					processGroup(t, f.e, f.at.Add(time.Hour), false)
				} else {
					processGroup(t, f.e, f.at.Add(time.Hour), true)
					require.Equal(t, "firing", (<-f.received).Status)
					processGroup(t, f.e, f.at.Add(2*time.Hour), false)
				}
			})
		}
	}
}

func TestNotificationCooldownNewReceiverGetsItsFirstFiringImmediately(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		t.Run(strconv.FormatBool(grouped), func(t *testing.T) {
			f := newCooldownFixture(t, grouped)
			evaluateGroup(t, f.e, f.at, map[string]float64{"a": 1})
			processGroup(t, f.e, f.at, true)
			<-f.received
			evaluateGroup(t, f.e, f.at.Add(2*time.Second), map[string]float64{"a": 0})
			processGroup(t, f.e, f.at.Add(2*time.Second), true)
			<-f.received
			point, err := f.e.Store.ContactPoint(t.Context(), "webhook")
			require.NoError(t, err)
			point.UID, point.Name, point.Version = "another", "Another receiver", 0
			_, err = f.e.Store.SaveContactPoint(t.Context(), model.ContactPointInput{ContactPoint: point})
			require.NoError(t, err)
			policy, err := f.e.Store.NotificationPolicy(t.Context())
			require.NoError(t, err)
			policy.Receiver = "another"
			_, err = f.e.Store.SaveNotificationPolicy(t.Context(), policy)
			require.NoError(t, err)
			evaluateGroup(t, f.e, f.at.Add(4*time.Second), map[string]float64{"a": 1})
			processGroup(t, f.e, f.at.Add(4*time.Second), true)
			require.Equal(t, "firing", (<-f.received).Status)
			history, err := f.e.Store.Notifications(t.Context(), "another", 10)
			require.NoError(t, err)
			require.Len(t, history, 1)
			require.Equal(t, "sent", history[0].State)
		})
	}
}

func TestNotificationCooldownRecoveryReceiptSurvivesOutboxRetention(t *testing.T) {
	f := newCooldownFixture(t, false)
	evaluateGroup(t, f.e, f.at, map[string]float64{"a": 1})
	processGroup(t, f.e, f.at, true)
	first := <-f.received
	// Simulate terminal-history retention independently of receipt metadata.
	_, err := f.e.Store.DB.Exec(`DELETE FROM alert_notification_outbox WHERE id=?`, first.ID)
	require.NoError(t, err)
	// Another instance succeeds after the maximum CD age. The still-active
	// first period's receipt must survive metadata pruning as well.
	evaluateGroup(t, f.e, f.at.Add(8*24*time.Hour), map[string]float64{"a": 1, "b": 1})
	processGroup(t, f.e, f.at.Add(8*24*time.Hour), true)
	<-f.received
	var receipts int
	require.NoError(t, f.e.Store.DB.QueryRow(`SELECT count(*) FROM alert_notification_cooldown_receipts`).Scan(&receipts))
	require.Equal(t, 2, receipts)
	evaluateGroup(t, f.e, f.at.Add(8*24*time.Hour+time.Second), map[string]float64{"a": 0, "b": 1})
	processGroup(t, f.e, f.at.Add(8*24*time.Hour+time.Second), true)
	recovery := <-f.received
	require.Equal(t, "resolved", recovery.Status)
	require.Equal(t, "a", recovery.Alerts[0].Labels["instance"])
}
