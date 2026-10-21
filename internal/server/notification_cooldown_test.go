package server_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/stretchr/testify/require"
)

func TestNotificationCooldownNativeAPIBoundsAndRoundTrip(t *testing.T) {
	_, handler := setup(t, "synthetic-fixed-token-233")
	for _, seconds := range []int{-1, 604801} {
		response := call(handler, "POST", "/api/v1/alerts/rules", fmt.Sprintf(`{"uid":"bad","title":"Bad cooldown","expr":"vector(1)","notification_cooldown_seconds":%d}`, seconds), "synthetic-fixed-token-233", "")
		require.Equal(t, 400, response.Code, response.Body.String())
	}
	for _, seconds := range []int{0, 3600, 604800} {
		t.Run(fmt.Sprint(seconds), func(t *testing.T) {
			_, handler := setup(t, "synthetic-fixed-token-233")
			response := call(handler, "POST", "/api/v1/alerts/rules", fmt.Sprintf(`{"uid":"cooldown","title":"Cooldown","expr":"vector(1)","notification_cooldown_seconds":%d}`, seconds), "synthetic-fixed-token-233", "")
			require.Equal(t, 200, response.Code, response.Body.String())
			response = call(handler, "GET", "/api/v1/alerts/rules/cooldown", "", "synthetic-fixed-token-233", "")
			require.Equal(t, 200, response.Code, response.Body.String())
			var rule model.AlertRuleView
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &rule))
			require.Equal(t, seconds, rule.NotificationCooldownSeconds)
		})
	}
}
