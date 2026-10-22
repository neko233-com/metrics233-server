package alerting

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/notificationtemplate"
	"github.com/stretchr/testify/require"
)

func feishuCardFixture(status, severity string) map[string]any {
	at := time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)
	alert := map[string]any{"status": status, "labels": map[string]any{"instance": "example-host", "resource": "cpu", "severity": severity, "validation_id": "private-validation-identifier", "managed_by": "private-manager"},
		"annotations": map[string]any{"title": "CPU 使用率持续高", "threshold": "> 85%", "unit": "%", "description": "检查实际工作负载。"},
		"startsAt":    at.Add(-20 * time.Minute).Format(time.RFC3339), "endsAt": "0001-01-01T00:00:00Z", "generatorURL": "https://untrusted.example/override",
		"values": map[string]any{"A": 88.5}, "rule": notificationtemplate.RuleMetadata{UID: "cpu-warning", Title: "Host CPU", Expr: "NEVER-DUMP-PROMQL-EXPRESSION", ForSeconds: 600, KeepFiringForSeconds: 300, NotificationCooldownSeconds: 3600}}
	if status == "resolved" {
		alert["endsAt"] = at.Format(time.RFC3339)
	}
	return map[string]any{"status": status, "title": "Host CPU", "message": "", "notificationId": "42", "externalURL": "https://untrusted.example/override", "alerts": []any{alert}}
}

func TestFeishuCardDefaultsColorsRuleFactsAndNoRawLabelDump(t *testing.T) {
	for _, item := range []struct{ status, severity, color string }{{"firing", "critical", "red"}, {"firing", "warning", "orange"}, {"resolved", "critical", "green"}} {
		t.Run(item.status+item.severity, func(t *testing.T) {
			root, _ := url.Parse("https://monitoring.example.com/base")
			body, err := RenderFeishuNotification(feishuCardFixture(item.status, item.severity), "", "", root, time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC))
			require.NoError(t, err)
			var value map[string]any
			require.NoError(t, json.Unmarshal(body, &value))
			require.Equal(t, "interactive", value["msg_type"])
			card := value["card"].(map[string]any)
			require.Equal(t, item.color, card["header"].(map[string]any)["template"])
			for _, expected := range []string{"example-host", "CPU", "88.50%", "10分钟", "5分钟", "1小时", "85%", "北京时间", "18:00:00", "最近 60 分钟", "https://monitoring.example.com/base/#alerts"} {
				require.Contains(t, string(body), expected)
			}
			for _, hidden := range []string{"private-validation-identifier", "private-manager", "NEVER-DUMP-PROMQL-EXPRESSION", "https://untrusted.example", "lark_md", "markdown"} {
				require.NotContains(t, string(body), hidden)
			}
		})
	}
}

func TestFeishuCardPlainTextSinglePassUntrustedMentionLinksAndUTF8Budget(t *testing.T) {
	payload := feishuCardFixture("firing", "warning")
	attack := `<at user_id="all">everyone</at> [click](https://attacker.example) {{ .Receiver }} @all`
	payload["title"] = attack
	payload["message"] = attack
	alerts := payload["alerts"].([]any)
	alert := alerts[0].(map[string]any)
	alert["labels"].(map[string]any)["instance"] = attack
	alert["annotations"].(map[string]any)["description"] = strings.Repeat("飞书\x01"+attack, 300)
	for range 12 {
		alerts = append(alerts, alert)
	}
	payload["alerts"] = alerts
	for _, format := range []string{"interactive", "text"} {
		body, err := RenderFeishuNotification(payload, format, "synthetic-key", nil, time.Now())
		require.NoError(t, err)
		require.LessOrEqual(t, len(body), 20*1024)
		require.True(t, utf8.Valid(body))
		var encoded any
		require.NoError(t, json.Unmarshal(body, &encoded))
		flat, _ := json.Marshal(encoded)
		require.NotContains(t, string(flat), "https://attacker.example")
		require.NotContains(t, string(flat), `\u003cat`)
		require.NotContains(t, string(flat), "@all")
		require.Contains(t, string(flat), "{{ .Receiver }}", "data is never parsed as template source")
		require.NotContains(t, string(flat), `"url":`)
	}
}

func TestFeishuCardAcceptanceIsExplicitAndNeverMisstatesZeroUsage(t *testing.T) {
	payload := feishuCardFixture("resolved", "info")
	alert := payload["alerts"].([]any)[0].(map[string]any)
	alert["labels"].(map[string]any)["scope"] = "acceptance"
	alert["values"] = map[string]any{"A": 0}
	body, err := RenderFeishuNotification(payload, "interactive", "", nil, time.Now())
	require.NoError(t, err)
	require.Contains(t, string(body), "非资源故障")
	require.Contains(t, string(body), "通知测试结束")
	require.NotContains(t, string(body), "A=0")
	require.NotContains(t, string(body), "验收时间窗")
	delete(alert["labels"].(map[string]any), "scope")
	body, err = RenderFeishuNotification(payload, "interactive", "", nil, time.Now())
	require.NoError(t, err)
	require.Contains(t, string(body), "已满足恢复条件")
	require.NotContains(t, string(body), "A=0.00%")
	for _, raw := range []string{"http://monitoring.example", "https://user:pass@monitoring.example", "https://monitoring.example?secret=x", "https://monitoring.example#untrusted"} {
		root, _ := url.Parse(raw)
		require.Empty(t, trustedFeishuAlertURL(root))
	}
}

func TestFeishuCardReadableSnapshotAndCustomText(t *testing.T) {
	payload := feishuCardFixture("firing", "critical")
	payload["notificationCreatedAt"] = "2026-10-21T15:59:59Z"
	payload["message"] = "请值班同学检查数据库连接池。"
	alert := payload["alerts"].([]any)[0].(map[string]any)
	alert["annotations"].(map[string]any)["value_label"] = "触发时使用率"
	body, err := RenderFeishuNotification(payload, "", "", nil, time.Date(2026, 10, 22, 2, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	for _, expected := range []string{"持续超限", "触发时使用率 · 88.50%", "23:59:59", "请值班同学检查数据库连接池。", `通知冷却\n1小时`} {
		require.Contains(t, string(body), expected)
	}
	for _, hidden := range []string{"FIRING", "RESOLVED", "warning", "critical", "CD ", "UTC", "A=", "\\u003cnil\\u003e"} {
		require.NotContains(t, string(body), hidden)
	}
	body, err = RenderFeishuNotification(map[string]any{"status": "test", "title": "飞书连接测试"}, "", "", nil, time.Now())
	require.NoError(t, err)
	require.Contains(t, string(body), "通知测试")
	require.NotContains(t, string(body), "持续超限")
	payload = feishuCardFixture("firing", "warning")
	alert = payload["alerts"].([]any)[0].(map[string]any)
	alert["annotations"].(map[string]any)["summary"] = "需要保留的自定义说明"
	payload["commonAnnotations"] = map[string]any{"summary": "需要保留的自定义说明"}
	payload["message"] = "需要保留的自定义说明"
	body, err = RenderFeishuNotification(payload, "", "", nil, time.Now())
	require.NoError(t, err)
	require.Contains(t, string(body), "需要保留的自定义说明")
}

func TestFeishuContactPointTestUsesTestCardInsteadOfResourceAlarm(t *testing.T) {
	ctx := context.Background()
	s := notificationStore(t)
	_, err := s.SaveContactPoint(ctx, model.ContactPointInput{ContactPoint: model.ContactPoint{UID: "readable-test", Name: "Operations", Type: "feishu", URL: syntheticFeishuURL}})
	require.NoError(t, err)
	queued, err := s.EnqueueTestNotification(ctx, "readable-test")
	require.NoError(t, err)
	var received string
	e := &Engine{Store: s, notificationTransport: localFeishuTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		received = string(body)
		_, _ = io.WriteString(w, `{"code":0}`)
	}))}
	processed, err := e.ProcessNotification(ctx, time.Now().Add(time.Second))
	require.NoError(t, err)
	require.True(t, processed)
	require.Contains(t, received, "通知测试 · 飞书通知")
	require.Contains(t, received, "无需处理")
	require.NotContains(t, received, "持续超限")
	require.NotContains(t, received, "Test notification")
	items, err := s.Notifications(ctx, "readable-test", 10)
	require.NoError(t, err)
	require.Equal(t, queued.ID, items[0].ID)
	require.Equal(t, "sent", items[0].State)
}

func TestFeishuCardTLSGroupedFiringResolvedAndCooldownRequireFullACK(t *testing.T) {
	ctx := context.Background()
	s := notificationStore(t)
	_, err := s.SaveContactPoint(ctx, model.ContactPointInput{ContactPoint: model.ContactPoint{UID: "card", Name: "Card", Type: "feishu", URL: syntheticFeishuURL}})
	require.NoError(t, err)
	group := []string{"instance", "severity"}
	_, err = s.SaveNotificationPolicy(ctx, model.NotificationPolicy{Receiver: "card", NotificationTiming: model.NotificationTiming{GroupBy: &group, GroupWait: "0s", GroupInterval: "1s", RepeatInterval: "10s"}})
	require.NoError(t, err)
	rule, err := s.SaveAlertRule(ctx, model.AlertRule{UID: "card-rule", Title: "Resource", Expr: "vector(1)", Condition: "nonzero", NotificationCooldownSeconds: 3600, Annotations: map[string]string{"threshold": "> 85%", "unit": "%"}})
	require.NoError(t, err)
	var calls atomic.Int32
	received := make(chan map[string]any, 8)
	e := &Engine{Store: s, active: map[string]bool{}, slots: make(chan struct{}, 4)}
	e.notificationTransport = localFeishuTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var card map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&card))
		received <- card
		if calls.Add(1) == 1 {
			_, _ = io.WriteString(w, `{"code":null}`)
		} else {
			_, _ = io.WriteString(w, `{"code":0}`)
		}
	}))
	current := 88.0
	e.Query = func(context.Context, string, time.Time) ([]Value, error) {
		out := []Value{}
		for _, resource := range []string{"cpu", "memory", "disk"} {
			out = append(out, Value{Value: current, Labels: map[string]string{"instance": "host", "severity": "warning", "resource": resource}})
		}
		return out, nil
	}
	at := time.Now().Add(time.Hour).Truncate(time.Second)
	_, err = e.Evaluate(ctx, rule.UID, at)
	require.NoError(t, err)
	processGroup(t, e, at, true)
	first := <-received
	require.Equal(t, "interactive", first["msg_type"])
	history, err := s.Notifications(ctx, "card", 10)
	require.NoError(t, err)
	require.Equal(t, "pending", history[0].State)
	processGroup(t, e, at.Add(5*time.Second), true)
	second := <-received
	require.Equal(t, first["card"], second["card"], "retry card content stays stable")
	processGroup(t, e, at.Add(20*time.Second), false)
	current = 0
	_, err = e.Evaluate(ctx, rule.UID, at.Add(30*time.Second))
	require.NoError(t, err)
	processGroup(t, e, at.Add(31*time.Second), true)
	resolved := <-received
	require.Equal(t, "green", resolved["card"].(map[string]any)["header"].(map[string]any)["template"])
	current = 90
	_, err = e.Evaluate(ctx, rule.UID, at.Add(50*time.Second))
	require.NoError(t, err)
	processGroup(t, e, at.Add(51*time.Second), false)
	current = 0
	_, err = e.Evaluate(ctx, rule.UID, at.Add(60*time.Second))
	require.NoError(t, err)
	processGroup(t, e, at.Add(61*time.Second), false)
	require.Empty(t, received)
	require.EqualValues(t, 3, calls.Load())
	history, err = s.Notifications(ctx, "card", 10)
	require.NoError(t, err)
	for _, delivery := range history {
		if delivery.State == "sent" {
			require.Equal(t, 200, delivery.HTTPStatus)
		}
	}
}
