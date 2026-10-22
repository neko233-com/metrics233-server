package alerting

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/neko233-com/metrics233-server/internal/notificationtemplate"
)

// RenderFeishuNotification only formats already-rendered data. It never parses
// label/annotation values as templates or accepts user-supplied card JSON.
// trustedRoot comes from server configuration, never a label or generatorURL.
func RenderFeishuNotification(payload map[string]any, format, secret string, trustedRoot *url.URL, at time.Time) ([]byte, error) {
	if format == "text" {
		return feishuNotificationPayload(payload, secret, at)
	}
	if format != "" && format != "interactive" {
		return nil, errors.New("invalid Feishu format")
	}
	if payload["status"] == nil || payload["status"] == "" {
		payload["status"] = "test"
	}
	isTest := payload["status"] == "test"
	data, err := notificationtemplate.DataFromPayload(payload, at)
	if err != nil {
		return nil, err
	}
	createdAt := at
	if value, _ := payload["notificationCreatedAt"].(string); value != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			createdAt = parsed
		}
	}
	status, color := "持续超限", "orange"
	if data.Status == "resolved" {
		status, color = "已恢复", "green"
	}
	acceptance := len(data.Alerts) > 0
	for _, alert := range data.Alerts {
		if alert.Labels["scope"] != "acceptance" {
			acceptance = false
		}
		if data.Status != "resolved" && alert.Status == "firing" && strings.EqualFold(alert.Labels["severity"], "critical") {
			color = "red"
		}
	}
	if acceptance || isTest {
		status, color = "通知测试", "blue"
		if data.Status == "resolved" {
			status = "通知测试结束"
		}
	}
	title, _ := payload["title"].(string)
	if title == "" {
		title = "Metrics233"
	}
	title = strings.TrimPrefix(strings.TrimPrefix(title, "[FIRING] "), "[RESOLVED] ")
	if isTest && title == "Test notification" {
		title = "飞书通知"
	}
	if strings.HasPrefix(title, "["+strings.ToUpper(data.Status)+":") {
		title = "主机监控"
	}
	var body []byte
	// Rebuild with progressively smaller text blocks when JSON escaping expands
	// content. Structural fields/signature survive and valid UTF-8 is preserved.
	for budget := 1000; budget >= 125; budget /= 2 {
		elements := []any{}
		host := firstFeishu(data.CommonLabels["host"], data.CommonLabels["hostname"], data.CommonLabels["instance"], data.CommonLabels["serverId"])
		if host == "" {
			host = "按资源查看主机"
		}
		heading := host + " · " + strconv.Itoa(len(data.Alerts)) + " 项"
		if isTest || acceptance {
			heading = "飞书通知通道"
		}
		elements = append(elements, feishuDiv(heading, budget))
		if message, _ := payload["message"].(string); message != "" {
			shown := false
			if !acceptance && !isTest {
				for i, alert := range data.Alerts {
					if i < 6 && message == feishuAlertName(alert) {
						shown = true
					}
				}
			}
			if !shown {
				elements = append(elements, feishuDiv(message, budget))
			}
		}
		if acceptance || isTest {
			elements = append(elements, feishuDiv("这是一条通知测试，非资源故障，无需处理。\n正式告警仅在满足持续触发条件后发送。", budget))
		}
		for i, alert := range data.Alerts {
			if acceptance || isTest {
				break
			}
			if i >= 6 {
				elements = append(elements, feishuDiv(fmt.Sprintf("另有 %d 项资源；请在告警页面查看完整分组。", len(data.Alerts)-i), budget))
				break
			}
			name := feishuAlertName(alert)
			peer := firstFeishu(alert.Labels["host"], alert.Labels["hostname"], alert.Labels["instance"], alert.Labels["serverId"], host)
			if mount := alert.Labels["mountpoint"]; mount != "" {
				peer += " · " + mount
			}
			if i > 0 {
				elements = append(elements, map[string]any{"tag": "hr"})
			}
			elements = append(elements, feishuDiv(name+" · "+feishuSeverity(alert.Labels["severity"]), min(budget, 240)),
				feishuFields([][2]string{{"主机", peer}, {"当前观测", feishuActualValue(alert)}}, budget))
			threshold := firstFeishu(alert.Annotations["threshold"], alert.Annotations["condition"], "见规则详情")
			if alert.Status == "resolved" {
				threshold = firstFeishu(alert.Annotations["recovery_threshold"], "已满足恢复条件；原告警线 "+threshold)
			}
			end := createdAt
			if !alert.EndsAt.IsZero() {
				end = alert.EndsAt
			}
			duration := "未记录"
			if !alert.StartsAt.IsZero() && !end.Before(alert.StartsAt) {
				duration = feishuDuration(int(end.Sub(alert.StartsAt).Seconds()))
			}
			timeLabel, timeValue := "开始时间", feishuTime(alert.StartsAt)
			if alert.Status == "resolved" {
				timeLabel, timeValue = "恢复时间", feishuTime(alert.EndsAt)
			}
			cooldown := "未设置"
			if alert.Rule.NotificationCooldownSeconds > 0 {
				cooldown = feishuDuration(alert.Rule.NotificationCooldownSeconds)
			}
			elements = append(elements, feishuFields([][2]string{{"恢复条件", threshold}, {"本轮持续", duration}, {timeLabel, timeValue}, {"通知冷却", cooldown}}, min(budget, 240)))
			if alert.Status != "resolved" {
				// The label is different for firing, but the field layout stays stable.
				fields := elements[len(elements)-1].(map[string]any)["fields"].([]any)
				fields[0] = map[string]any{"is_short": true, "text": feishuPlain("告警条件\n"+threshold, min(budget, 240))}
			}
			timing := ""
			if alert.Rule.UID != "" {
				timing = "持续 " + feishuDuration(alert.Rule.ForSeconds) + " 后通知 · 恢复需连续 " + feishuDuration(alert.Rule.KeepFiringForSeconds)
			}
			if timing != "" {
				elements = append(elements, feishuDiv(timing, min(budget, 160)))
			}
			if action := firstFeishu(alert.Annotations["action"], alert.Annotations["description"]); action != "" {
				elements = append(elements, feishuDiv(action, min(budget, 240)))
			}

		}
		if link := trustedFeishuAlertURL(trustedRoot); link != "" {
			label := "查看告警详情"
			if dashboard := trustedFeishuDashboardURL(trustedRoot, data.Alerts); dashboard != "" {
				link = dashboard
				label = "查看主机仪表盘"
			}
			elements = append(elements, map[string]any{"tag": "action", "actions": []any{map[string]any{"tag": "button", "text": feishuPlain(label, 100), "type": "primary", "url": link}}})
		}
		note := "Metrics233 · " + feishuTime(createdAt) + " 北京时间 · 最近 60 分钟"
		if id := payload["notificationId"]; id != nil && fmt.Sprint(id) != "" {
			note += " · 通知 " + fmt.Sprint(id)
		}
		elements = append(elements, map[string]any{"tag": "note", "elements": []any{feishuPlain(note, 150)}})
		message := map[string]any{"msg_type": "interactive", "card": map[string]any{"config": map[string]any{"wide_screen_mode": true}, "header": map[string]any{"template": color, "title": feishuPlain(status+" · "+title, 200)}, "elements": elements}}
		feishuSign(message, secret, at)
		body, err = json.Marshal(message)
		if err != nil || len(body) <= 20*1024 {
			return body, err
		}
	}
	return nil, errors.New("Feishu card exceeds encoded message budget")
}

var feishuURLText = regexp.MustCompile(`(?i)https?://|www\.`)
var feishuDashboardUID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,40}$`)

func trustedFeishuDashboardURL(root *url.URL, alerts []notificationtemplate.Alert) string {
	if trustedFeishuAlertURL(root) == "" || len(alerts) == 0 {
		return ""
	}
	uid := alerts[0].Annotations["__dashboardUid__"]
	if !feishuDashboardUID.MatchString(uid) {
		return ""
	}
	for _, alert := range alerts {
		if alert.Annotations["__dashboardUid__"] != uid {
			return ""
		}
	}
	copy := *root
	copy.Path = strings.TrimRight(copy.Path, "/") + "/d/" + uid + "/host"
	copy.RawPath = ""
	copy.Fragment = ""
	return copy.String()
}

func safeFeishuText(value string, limit int) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, value)
	value = strings.NewReplacer("<", "＜", ">", "＞", "@all", "＠all").Replace(value)
	value = feishuURLText.ReplaceAllStringFunc(value, func(part string) string { return strings.NewReplacer(":", "：", ".", "．").Replace(part) })
	return boundedFeishuText(value, limit)
}
func feishuPlain(value string, limit int) map[string]string {
	return map[string]string{"tag": "plain_text", "content": safeFeishuText(value, limit)}
}
func feishuDiv(value string, limit int) map[string]any {
	return map[string]any{"tag": "div", "text": feishuPlain(value, limit)}
}
func feishuFields(fields [][2]string, limit int) map[string]any {
	out := []any{}
	for _, field := range fields {
		out = append(out, map[string]any{"is_short": true, "text": feishuPlain(field[0]+"\n"+field[1], limit)})
	}
	return map[string]any{"tag": "div", "fields": out}
}
func firstFeishu(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
func feishuAlertName(alert notificationtemplate.Alert) string {
	return firstFeishu(alert.Annotations["title"], alert.Annotations["summary"], alert.Rule.Title, alert.Labels["alertname"], feishuResource(alert.Labels["resource"]))
}
func feishuResource(value string) string {
	if name := map[string]string{"cpu": "CPU", "memory": "内存", "disk": "磁盘", "inode": "Inode", "inodes": "Inode", "up": "采集可用性"}[value]; name != "" {
		return name
	}
	return firstFeishu(value, "指标")
}
func feishuDuration(seconds int) string {
	if seconds != 0 && seconds%3600 == 0 {
		return fmt.Sprintf("%d小时", seconds/3600)
	}
	if seconds != 0 && seconds%60 == 0 {
		return fmt.Sprintf("%d分钟", seconds/60)
	}
	if seconds >= 60 {
		return fmt.Sprintf("%d分%d秒", seconds/60, seconds%60)
	}
	return fmt.Sprintf("%d秒", seconds)
}
func feishuTime(at time.Time) string {
	if at.IsZero() {
		return "未记录"
	}
	return at.In(time.FixedZone("CST", 8*60*60)).Format("2006-01-02 15:04:05")
}
func feishuActualValue(alert notificationtemplate.Alert) string {
	if alert.Labels["scope"] == "acceptance" && alert.Status == "resolved" {
		return "通知测试已结束"
	}
	if alert.Status == "resolved" && (alert.Labels["resource"] == "cpu" || alert.Labels["resource"] == "memory" || alert.Labels["resource"] == "disk") {
		if value, ok := alert.Values["A"]; ok && value == 0 && alert.Annotations["value_ref"] == "" {
			return "已满足恢复条件"
		}
	}
	// Graph rules keep the raw measurement separate from boolean condition refs.
	// A trusted rule annotation selects the numeric ref; labels never become code.
	if ref := alert.Annotations["value_ref"]; ref != "" {
		if value, ok := alert.Values[ref]; ok && !math.IsNaN(value) && !math.IsInf(value, 0) {
			return firstFeishu(alert.Annotations["value_label"], "使用率") + " · " + strconv.FormatFloat(value, 'f', 2, 64) + alert.Annotations["unit"]
		}
		return "当前值 · 暂无有效观测"
	}
	keys := make([]string, 0, len(alert.Values))
	for key := range alert.Values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := []string{}
	for _, key := range keys {
		if len(parts) == 4 {
			break
		}
		if value := alert.Values[key]; !math.IsNaN(value) && !math.IsInf(value, 0) {
			prefix := key + "="
			if len(alert.Values) == 1 && key == "A" {
				prefix = ""
			}
			parts = append(parts, prefix+strconv.FormatFloat(value, 'f', 2, 64))
		}
	}
	unit := alert.Annotations["unit"]
	value := firstFeishu(alert.Annotations["value_label"], "评估值") + " · " + strings.Join(parts, " · ") + unit
	if len(parts) == 0 {
		value = "规则评估值 · 未记录"
	}
	if alert.Labels["scope"] == "acceptance" {
		value += "（实测比例，验收时间窗）"
	}
	return value
}

func feishuSeverity(value string) string {
	if title := map[string]string{"warning": "警告", "critical": "严重", "info": "提示"}[strings.ToLower(value)]; title != "" {
		return title
	}
	return firstFeishu(value, "未指定严重级别")
}
func trustedFeishuAlertURL(root *url.URL) string {
	if root == nil || root.Scheme != "https" || root.Host == "" || root.User != nil || root.RawQuery != "" || root.Fragment != "" {
		return ""
	}
	copy := *root
	copy.Path = strings.TrimRight(copy.Path, "/") + "/"
	copy.RawPath = ""
	copy.Fragment = "alerts"
	return copy.String()
}
