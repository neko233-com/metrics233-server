package alerting

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Feishu's custom-bot v2 protocol signs the empty message, using
// timestamp + "\n" + secret as the HMAC key (not the request body).
// https://open.feishu.cn/document/client-docs/bot-v3/add-custom-bot
func feishuNotificationPayload(payload map[string]any, secret string, at time.Time) ([]byte, error) {
	text := feishuNotificationText(payload)
	message := map[string]any{"msg_type": "text"}
	if secret != "" {
		timestamp := strconv.FormatInt(at.Unix(), 10)
		sign := hmac.New(sha256.New, []byte(timestamp+"\n"+secret))
		message["timestamp"] = timestamp
		message["sign"] = base64.StdEncoding.EncodeToString(sign.Sum(nil))
	}
	for {
		message["content"] = map[string]string{"text": text}
		body, err := json.Marshal(message)
		if err != nil || len(body) <= 20*1024 {
			return body, err
		}
		// JSON escaping can expand control characters. Bound the encoded body,
		// preserving valid UTF-8 instead of relying only on a source-text cap.
		text = boundedFeishuText(text, len(text)*3/4)
	}
}

func boundedFeishuText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	if limit < 3 {
		return "..."
	}
	end := limit - 3
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end] + "..."
}

func feishuNotificationText(payload map[string]any) string {
	var out strings.Builder
	appendText := func(value string) {
		if out.Len() < 16000 {
			out.WriteString(boundedFeishuText(value, 16000-out.Len()))
		}
	}
	title, _ := payload["title"].(string)
	if title == "" {
		title = "Metrics233 · " + fmt.Sprint(payload["status"])
	}
	appendText(title + "\n")
	if message, _ := payload["message"].(string); message != "" {
		appendText(message + "\n")
	}
	if alerts, ok := payload["alerts"].([]any); ok {
		for i, item := range alerts {
			if i == 20 {
				appendText("Additional alerts omitted\n")
				break
			}
			alert, ok := item.(map[string]any)
			if !ok {
				continue
			}
			for _, field := range []string{"labels", "annotations", "values"} {
				values, ok := alert[field].(map[string]any)
				if !ok {
					continue
				}
				keys := make([]string, 0, len(values))
				for key := range values {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				for _, key := range keys {
					appendText(key + "=" + fmt.Sprint(values[key]) + "\n")
				}
			}
		}
	}
	if external, _ := payload["externalURL"].(string); external != "" {
		appendText(external + "\n")
	}
	appendText("Notification: " + fmt.Sprint(payload["notificationId"]))
	return out.String()
}
