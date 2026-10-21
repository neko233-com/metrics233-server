package alerting

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/stretchr/testify/require"
)

const syntheticFeishuURL = "https://open.feishu.cn/open-apis/bot/v2/hook/synthetic-fixture-only"
const syntheticFeishuSecret = "synthetic-signing-secret-only"

type feishuFixtureTransport func(*http.Request) (*http.Response, error)

func (f feishuFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func localFeishuTransport(t *testing.T, receiver http.Handler) http.RoundTripper {
	t.Helper()
	server := httptest.NewTLSServer(receiver)
	t.Cleanup(server.Close)
	local, err := url.Parse(server.URL)
	require.NoError(t, err)
	return feishuFixtureTransport(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != syntheticFeishuURL {
			t.Error("unexpected notification destination")
		}
		request = request.Clone(request.Context())
		copyURL := *request.URL
		copyURL.Scheme, copyURL.Host = local.Scheme, local.Host
		request.URL = &copyURL
		return server.Client().Transport.RoundTrip(request)
	})
}

func TestFeishuEncryptedCredentialsPreserveClearAndValidate(t *testing.T) {
	ctx := context.Background()
	s := notificationStore(t)
	secret := syntheticFeishuSecret
	point, err := s.SaveContactPoint(ctx, model.ContactPointInput{ContactPoint: model.ContactPoint{UID: "feishu", Name: "Feishu synthetic", Type: "feishu", URL: syntheticFeishuURL}, SigningSecret: &secret})
	require.NoError(t, err)
	require.Empty(t, point.URL)
	require.Equal(t, map[string]bool{"webhook_url": true, "signing_secret": true}, point.SecureFields)
	var public string
	var sealed []byte
	require.NoError(t, s.DB.QueryRow(`SELECT config,secrets FROM alert_contact_points WHERE uid='feishu'`).Scan(&public, &sealed))
	for _, raw := range []string{public, string(sealed)} {
		require.NotContains(t, raw, syntheticFeishuURL)
		require.NotContains(t, raw, syntheticFeishuSecret)
	}
	point.Name = "Renamed"
	point, err = s.SaveContactPoint(ctx, model.ContactPointInput{ContactPoint: point})
	require.NoError(t, err)
	secrets, err := s.ContactPointSecrets(ctx, point.UID)
	require.NoError(t, err)
	require.Equal(t, syntheticFeishuURL, secrets["webhook_url"])
	require.Equal(t, secret, secrets["signing_secret"])
	_, err = s.SaveContactPoint(ctx, model.ContactPointInput{ContactPoint: point, ClearSecrets: []string{"webhook_url"}})
	require.Error(t, err, "cannot commit a point with no destination")
	point, err = s.SaveContactPoint(ctx, model.ContactPointInput{ContactPoint: point, ClearSecrets: []string{"signing_secret"}})
	require.NoError(t, err)
	require.False(t, point.SecureFields["signing_secret"])
	point, err = s.SaveContactPoint(ctx, model.ContactPointInput{ContactPoint: point, SigningSecret: &secret})
	require.NoError(t, err)
	empty := ""
	point, err = s.SaveContactPoint(ctx, model.ContactPointInput{ContactPoint: point, SigningSecret: &empty})
	require.NoError(t, err)
	require.False(t, point.SecureFields["signing_secret"])
	for _, endpoint := range []string{"", "http://open.feishu.cn/open-apis/bot/v2/hook/test", "https://open.feishu.cn.attacker.invalid/open-apis/bot/v2/hook/test", "https://open.feishu.cn/open-apis/bot/v2/hook/test?secret=bad", "https://open.feishu.cn:444/open-apis/bot/v2/hook/test", "https://user:password@open.feishu.cn/open-apis/bot/v2/hook/test", "https://open.feishu.cn/open-apis/bot/v2/hook/%74est"} {
		_, err := s.SaveContactPoint(ctx, model.ContactPointInput{ContactPoint: model.ContactPoint{Name: "Bad", Type: "feishu", URL: endpoint}})
		require.Error(t, err)
	}
	badSecret := "invalid\nheader"
	_, err = s.SaveContactPoint(ctx, model.ContactPointInput{ContactPoint: point, SigningSecret: &badSecret})
	require.Error(t, err)
	point.Type, point.URL = "webhook", "http://127.0.0.1/fixture"
	point, err = s.SaveContactPoint(ctx, model.ContactPointInput{ContactPoint: point})
	require.NoError(t, err)
	secrets, err = s.ContactPointSecrets(ctx, point.UID)
	require.NoError(t, err)
	require.Empty(t, secrets, "switching provider must not forward old credentials")
}

func TestFeishuSignatureAndEncodedMessageBudget(t *testing.T) {
	at := time.Unix(1599360473, 0)
	for _, message := range []string{"Synthetic test", strings.Repeat("飞书\x01<>&", 20000)} {
		body, err := feishuNotificationPayload(map[string]any{"title": "Alert", "message": message, "status": "test"}, "demo", at)
		require.NoError(t, err)
		require.LessOrEqual(t, len(body), 20*1024)
		var payload struct {
			Timestamp, Sign, MsgType string
			Content                  struct{ Text string }
		}
		// msg_type uses the protocol's snake_case key.
		var raw map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(body, &raw))
		require.NoError(t, json.Unmarshal(raw["timestamp"], &payload.Timestamp))
		require.NoError(t, json.Unmarshal(raw["sign"], &payload.Sign))
		require.NoError(t, json.Unmarshal(raw["msg_type"], &payload.MsgType))
		require.NoError(t, json.Unmarshal(raw["content"], &payload.Content))
		require.Equal(t, "1599360473", payload.Timestamp)
		require.Equal(t, "text", payload.MsgType)
		require.True(t, utf8.ValidString(payload.Content.Text))
		// Independently compute the official empty-message HMAC contract.
		mac := hmac.New(sha256.New, []byte("1599360473\ndemo"))
		require.Equal(t, base64.StdEncoding.EncodeToString(mac.Sum(nil)), payload.Sign)
	}
	body, err := feishuNotificationPayload(map[string]any{"status": "resolved"}, "", at)
	require.NoError(t, err)
	require.NotContains(t, string(body), `"sign"`)
	require.NotContains(t, string(body), `"timestamp"`)
}

func TestFeishuTLSQueueRequiresBusinessAcknowledgementAndRetainsRetry(t *testing.T) {
	ctx := context.Background()
	s := notificationStore(t)
	secret := syntheticFeishuSecret
	point, err := s.SaveContactPoint(ctx, model.ContactPointInput{ContactPoint: model.ContactPoint{UID: "feishu", Name: "Feishu synthetic", Type: "feishu", URL: syntheticFeishuURL, FeishuFormat: "text"}, SigningSecret: &secret})
	require.NoError(t, err)
	delivery, err := s.EnqueueTestNotification(ctx, point.UID)
	require.NoError(t, err)
	var calls atomic.Int32
	e := &Engine{Store: s}
	e.notificationTransport = localFeishuTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Authorization") != "" {
			t.Error("invalid Feishu transport headers")
		}
		var payload struct {
			Timestamp string `json:"timestamp"`
			Sign      string `json:"sign"`
			MsgType   string `json:"msg_type"`
			Content   struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		if json.NewDecoder(r.Body).Decode(&payload) != nil {
			t.Error("invalid notification JSON")
		}
		seconds, err := strconv.ParseInt(payload.Timestamp, 10, 64)
		if err != nil || time.Since(time.Unix(seconds, 0)) > time.Minute {
			t.Error("signature timestamp must be generated at delivery time")
		}
		mac := hmac.New(sha256.New, []byte(payload.Timestamp+"\n"+secret))
		if payload.Sign != base64.StdEncoding.EncodeToString(mac.Sum(nil)) || payload.MsgType != "text" || payload.Content.Text == "" {
			t.Error("invalid signed Feishu text")
		}
		w.Header().Set("Content-Type", "application/json")
		if calls.Add(1) == 1 {
			_, _ = io.WriteString(w, `{"code":19021,"msg":"synthetic-signing-secret-only"}`)
		} else {
			_, _ = io.WriteString(w, `{"code":0,"msg":"success"}`)
		}
	}))
	at := time.Now()
	processed, err := e.ProcessNotification(ctx, at)
	require.NoError(t, err)
	require.True(t, processed)
	history, err := s.Notifications(ctx, point.UID, 100)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, "pending", history[0].State)
	require.Equal(t, "Feishu rejected notification (code 19021)", history[0].LastError)
	require.NotContains(t, history[0].LastError, secret)
	processed, err = e.ProcessNotification(ctx, at.Add(10*time.Second))
	require.NoError(t, err)
	require.True(t, processed)
	history, err = s.Notifications(ctx, point.UID, 100)
	require.NoError(t, err)
	require.Equal(t, delivery.ID, history[0].ID)
	require.Equal(t, "sent", history[0].State)
	require.Equal(t, 2, history[0].Attempts)
	var savedPayload string
	require.NoError(t, s.DB.QueryRow(`SELECT payload FROM alert_notification_outbox WHERE id=?`, delivery.ID).Scan(&savedPayload))
	require.NotContains(t, savedPayload, syntheticFeishuURL)
	require.NotContains(t, savedPayload, secret)
}

func TestFeishuRejectsInvalidAcknowledgementsAndRedirects(t *testing.T) {
	for _, ack := range []string{`{}`, `{"code":null}`, `{"code":"0"}`, `{"StatusCode":0}`, `{"code":0} trailing`, strings.Repeat("x", 9000), `{"code":11232,"msg":"private-value"}`} {
		t.Run(strconv.Itoa(len(ack))+ack[:min(len(ack), 10)], func(t *testing.T) {
			e := &Engine{notificationTransport: localFeishuTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, ack) }))}
			code, message := e.deliverNotification(context.Background(), model.NotificationDelivery{}, model.ContactPoint{Type: "feishu", TimeoutSeconds: 1}, map[string]string{"webhook_url": syntheticFeishuURL}, json.RawMessage(`{"status":"test"}`), false)
			require.Equal(t, 200, code)
			require.NotEmpty(t, message)
			require.NotContains(t, message, "private-value")
		})
	}
	var redirects atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirects.Add(1) }))
	defer target.Close()
	e := &Engine{notificationTransport: localFeishuTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))}
	code, message := e.deliverNotification(context.Background(), model.NotificationDelivery{}, model.ContactPoint{Type: "feishu", TimeoutSeconds: 1}, map[string]string{"webhook_url": syntheticFeishuURL}, json.RawMessage(`{}`), false)
	require.Equal(t, 302, code)
	require.NotEmpty(t, message)
	require.Zero(t, redirects.Load())
}

func TestFeishuTimeoutAndCancellation(t *testing.T) {
	e := &Engine{notificationTransport: localFeishuTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))}
	code, message := e.deliverNotification(context.Background(), model.NotificationDelivery{}, model.ContactPoint{Type: "feishu", TimeoutSeconds: 1}, map[string]string{"webhook_url": syntheticFeishuURL}, json.RawMessage(`{}`), false)
	require.Zero(t, code)
	require.Equal(t, "Webhook request timed out", message)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, message = e.deliverNotification(ctx, model.NotificationDelivery{}, model.ContactPoint{Type: "feishu", TimeoutSeconds: 1}, map[string]string{"webhook_url": syntheticFeishuURL}, json.RawMessage(`{}`), false)
	require.NotEmpty(t, message)
}

func TestFeishuTransportDeadlineBeforeContextTimerRemainsSanitized(t *testing.T) {
	for _, test := range []struct {
		name    string
		err     error
		message string
	}{
		{"client deadline", &url.Error{Op: "Post", URL: syntheticFeishuURL, Err: context.DeadlineExceeded}, "Webhook request timed out"},
		{"network deadline", &net.OpError{Op: "dial", Net: "tcp", Err: os.ErrDeadlineExceeded}, "Webhook request timed out"},
		{"cancellation", context.Canceled, "Webhook request failed"},
		{"ordinary transport", errors.New("synthetic-private-transport-value"), "Webhook request failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			e := &Engine{notificationTransport: feishuFixtureTransport(func(*http.Request) (*http.Response, error) { return nil, test.err })}
			code, message := e.deliverNotification(ctx, model.NotificationDelivery{}, model.ContactPoint{Type: "feishu", TimeoutSeconds: 30}, map[string]string{"webhook_url": syntheticFeishuURL}, json.RawMessage(`{}`), false)
			require.NoError(t, ctx.Err(), "classification must work before the caller's context expires")
			require.Zero(t, code)
			require.Equal(t, test.message, message)
			require.NotContains(t, message, syntheticFeishuURL)
			require.NotContains(t, message, "synthetic-private-transport-value")
		})
	}
}

func TestFeishuFiringAndResolvedUseExistingAlertPolicy(t *testing.T) {
	ctx := context.Background()
	s := notificationStore(t)
	secret := syntheticFeishuSecret
	point, err := s.SaveContactPoint(ctx, model.ContactPointInput{ContactPoint: model.ContactPoint{UID: "feishu", Name: "Feishu synthetic", Type: "feishu", URL: syntheticFeishuURL, FeishuFormat: "text"}, SigningSecret: &secret})
	require.NoError(t, err)
	_, err = s.SaveNotificationPolicy(ctx, model.NotificationPolicy{Receiver: point.UID})
	require.NoError(t, err)
	rule, err := s.SaveAlertRule(ctx, model.AlertRule{UID: "feishu-business", Title: "Business queue", Expr: "vector(1)", Condition: "nonzero", Annotations: map[string]string{"summary": "Queue changed"}})
	require.NoError(t, err)
	received := make(chan string, 2)
	e := &Engine{Store: s, active: map[string]bool{}, slots: make(chan struct{}, 4), Query: func(context.Context, string, time.Time) ([]Value, error) {
		return []Value{{Value: 1, Labels: map[string]string{"service": "synthetic-shop"}}}, nil
	}}
	e.notificationTransport = localFeishuTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Content struct{ Text string } `json:"content"`
		}
		if json.NewDecoder(r.Body).Decode(&payload) != nil {
			t.Error("invalid Feishu text")
		}
		received <- payload.Content.Text
		_, _ = io.WriteString(w, `{"code":0}`)
	}))
	at := time.Now().Add(-time.Second)
	_, err = e.Evaluate(ctx, rule.UID, at)
	require.NoError(t, err)
	e.Query = func(context.Context, string, time.Time) ([]Value, error) {
		return []Value{{Value: 0, Labels: map[string]string{"service": "synthetic-shop"}}}, nil
	}
	_, err = e.Evaluate(ctx, rule.UID, at.Add(time.Second))
	require.NoError(t, err)
	for range 2 {
		processed, err := e.ProcessNotification(ctx, at.Add(2*time.Second))
		require.NoError(t, err)
		require.True(t, processed)
	}
	first, second := <-received, <-received
	require.Contains(t, first, "FIRING")
	require.Contains(t, first, "Business queue")
	require.Contains(t, first, "service=synthetic-shop")
	require.Contains(t, first, "A=1")
	require.Contains(t, second, "RESOLVED")
	history, err := s.Notifications(ctx, point.UID, 10)
	require.NoError(t, err)
	require.Len(t, history, 2)
	for _, delivery := range history {
		require.Equal(t, "sent", delivery.State)
	}
}
