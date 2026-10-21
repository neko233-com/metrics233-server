package server_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/stretchr/testify/require"
)

func TestFeishuContactPointAPIWriteOnlySecretsAndTestQueue(t *testing.T) {
	const token = "synthetic-admin-workspace-token"
	const endpoint = "https://open.feishu.cn/open-apis/bot/v2/hook/synthetic-api-fixture-only"
	const secret = "synthetic-api-signing-secret"
	db, handler := setup(t, token)
	body := `{"uid":"feishu-api","name":"Feishu","type":"feishu","url":"` + endpoint + `","signing_secret":"` + secret + `"}`
	for _, denied := range []string{"", "wrong-token"} {
		require.Equal(t, 401, call(handler, "POST", "/api/v1/alerting/contact-points", body, denied, "").Code)
	}
	response := call(handler, "POST", "/api/v1/alerting/contact-points", body, token, "")
	require.Equal(t, 201, response.Code, response.Body.String())
	var point model.ContactPoint
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &point))
	require.Empty(t, point.URL)
	require.True(t, point.SecureFields["webhook_url"])
	require.True(t, point.SecureFields["signing_secret"])
	for _, path := range []string{"/api/v1/alerting/contact-points", "/api/v1/alerting/contact-points/feishu-api", "/api/v1/provisioning/contact-points"} {
		response := call(handler, "GET", path, "", token, "")
		require.Equal(t, 200, response.Code)
		require.NotContains(t, response.Body.String(), endpoint)
		require.NotContains(t, response.Body.String(), secret)
	}
	point.Name = "Retain credentials"
	raw, err := json.Marshal(point)
	require.NoError(t, err)
	response = call(handler, "PUT", "/api/v1/alerting/contact-points/feishu-api", string(raw), token, "")
	require.Equal(t, 200, response.Code, response.Body.String())
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &point))
	secrets, err := db.ContactPointSecrets(context.Background(), point.UID)
	require.NoError(t, err)
	require.Equal(t, endpoint, secrets["webhook_url"])
	require.Equal(t, secret, secrets["signing_secret"])
	require.Equal(t, 409, call(handler, "PUT", "/api/v1/alerting/contact-points/feishu-api", string(raw), token, "").Code)
	require.Equal(t, 401, call(handler, "POST", "/api/v1/alerting/contact-points/feishu-api/test", "", "", "").Code)
	response = call(handler, "POST", "/api/v1/alerting/contact-points/feishu-api/test", "", token, "")
	require.Equal(t, 202, response.Code, response.Body.String())
	response = call(handler, "GET", "/api/v1/alerting/notifications", "", token, "")
	require.NotContains(t, response.Body.String(), secret)
	require.NotContains(t, response.Body.String(), endpoint)
}
