package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/neko233-com/metrics233-server/internal/alerting"
	"github.com/neko233-com/metrics233-server/internal/notificationtemplate"
)

type feishuPreviewInput struct {
	Format  string `json:"feishu_format"`
	Status  string `json:"status"`
	Title   string `json:"title_template"`
	Message string `json:"message_template"`
}

func (s *Server) feishuNotificationPreview(w http.ResponseWriter, r *http.Request) {
	var input feishuPreviewInput
	if err := decode(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	if input.Status == "" {
		input.Status = "firing"
	}
	if (input.Format != "" && input.Format != "interactive" && input.Format != "text") || (input.Status != "firing" && input.Status != "resolved") {
		fail(w, 400, errors.New("preview requires firing/resolved status and interactive/text format"))
		return
	}
	saved, err := s.Store.NotificationTemplates(r.Context())
	if err != nil {
		notificationError(w, err)
		return
	}
	definitions := make([]notificationtemplate.Definition, 0, len(saved))
	for _, value := range saved {
		definitions = append(definitions, notificationtemplate.Definition{Name: value.Name, Template: value.Template, Version: value.Version})
	}
	compiled, err := notificationtemplate.Compile(definitions)
	if err != nil {
		notificationError(w, err)
		return
	}
	at := time.Now().UTC()
	data := feishuPreviewData(input.Status, at)
	data.OrgID = s.OrganizationID()
	if s.Alerts != nil && s.Alerts.TemplateURL != nil {
		data.ExternalURL = s.Alerts.TemplateURL.String()
	}
	if err = data.Normalize(at); err != nil {
		notificationError(w, err)
		return
	}
	encoded, _ := json.Marshal(data)
	payload := map[string]any{}
	_ = json.Unmarshal(encoded, &payload)
	payload["title"] = "主机资源告警 · 示例预览"
	payload["notificationId"] = "preview"
	payload["message"] = "示例数据，仅用于格式预览，不评估真实资源，不会发送群消息。"
	renderCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	for _, field := range []struct{ name, source string }{{"title", input.Title}, {"message", input.Message}} {
		if field.source == "" {
			continue
		}
		output, err := compiled.Render(renderCtx, field.source, data)
		if err != nil {
			notificationError(w, err)
			return
		}
		payload[field.name] = output
	}
	var root *url.URL
	if s.Alerts != nil {
		root = s.Alerts.TemplateURL
	}
	body, err := alerting.RenderFeishuNotification(payload, input.Format, "", root, at)
	if err != nil {
		notificationError(w, err)
		return
	}
	format := input.Format
	if format == "" {
		format = "interactive"
	}
	writeJSON(w, 200, map[string]any{"format": format, "message": json.RawMessage(body), "bytes": len(body), "sent": false})
}

func feishuPreviewData(status string, at time.Time) notificationtemplate.Data {
	data := notificationtemplate.Data{Receiver: "示例飞书联系点", Status: status, GroupLabels: notificationtemplate.KV{"instance": "example-host"}, CommonLabels: notificationtemplate.KV{"instance": "example-host", "severity": "warning"}}
	for _, resource := range []struct {
		name, title string
		value       float64
	}{{"cpu", "CPU 使用率持续偏高", 88.5}, {"memory", "内存使用率持续偏高", 90.1}, {"disk", "根分区空间使用率持续偏高", 87.2}} {
		alert := notificationtemplate.Alert{Status: status, Labels: notificationtemplate.KV{"instance": "example-host", "severity": "warning", "resource": resource.name},
			Annotations: notificationtemplate.KV{"title": resource.title, "threshold": "> 85%", "unit": "%", "description": "仅示例预览，不代表实际主机负载。"}, Values: map[string]float64{"A": resource.value}, StartsAt: at.Add(-20 * time.Minute),
			Rule: notificationtemplate.RuleMetadata{UID: "preview-" + resource.name, Title: resource.title, Expr: "preview only", ForSeconds: 600, KeepFiringForSeconds: 300, NotificationCooldownSeconds: 3600}}
		if status == "resolved" {
			alert.EndsAt = at
			alert.Values["A"] = 65
		}
		data.Alerts = append(data.Alerts, alert)
	}
	return data
}
