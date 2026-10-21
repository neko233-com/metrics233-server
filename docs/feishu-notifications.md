# 飞书群机器人通知

联系点支持 `webhook` 和 `feishu`。飞书复用已有告警评估、通知策略、分组、静默、恢复通知及持久化重试队列；不需要额外依赖或飞书应用账号。

## 配置与凭据

在 Notifications → Contact points 新建联系点，类型选择 **Feishu**，输入群自定义机器人的 v2 Webhook 地址及可选签名密钥。机器人启用签名校验时必须填写匹配的密钥。使用关键词/IP 白名单时，仍需让实际消息和服务器出口满足飞书侧设置。

Webhook 地址包含凭据，和 `signing_secret` 一起使用现有 `secrets.key` 加密保存；GET/保存响应只返回 `secure_fields.webhook_url` / `secure_fields.signing_secret` 标记，`url` 为空。日志、通知历史及队列载荷不保存地址、密钥或飞书响应正文。数据库备份须同时保留匹配的 `secrets.key`。

只支持 `https://open.feishu.cn/open-apis/bot/v2/hook/<bot-id>`，不接受其他主机、端口、用户信息、查询参数或片段。通用 HTTP 接收端继续使用 `webhook` 类型。Feishu 不接受通用 Bearer token/自定义认证头，也不会把旧类型的凭据带到新类型。

## HTTP API

原生 API 使用 `POST /api/v1/alerting/contact-points` 创建，`PUT /api/v1/alerting/contact-points/{uid}` 更新。下例均为无效的合成占位符，实际值只应从私有配置输入：

```json
{
  "uid": "operations-feishu",
  "name": "Operations · Feishu",
  "type": "feishu",
  "url": "https://open.feishu.cn/open-apis/bot/v2/hook/synthetic-example-only",
  "signing_secret": "synthetic-example-only",
  "timeout_seconds": 10
}
```

更新必须携带最近读取的 `version`；冲突返回 409。已有 Feishu 联系点 `url` 为空时保留已加密的地址，省略 `signing_secret` 保留签名。`clear_secrets: ["signing_secret"]` 或显式 `signing_secret: ""` 删除签名；请同步飞书侧签名设置。切换类型需提供新类型的完整地址。创建和清除地址后没有有效目标时拒绝保存。

联系点保存、更新、删除、测试和通知重试在 local 模式下要求 Admin；Viewer/Editor 可读脱敏配置，不能更改或发送测试。token 模式下使用管理 `METRICSPANEL_TOKEN`；专用 `METRICSPANEL_INGEST_TOKEN` 只能上报指标，不能读写或测试联系点。

`POST /api/v1/alerting/contact-points/{uid}/test` 返回 202 表示**进入队列**，不表示飞书已收到。通过 `GET /api/v1/alerting/notifications?contact_point_uid={uid}` 查看 `pending/sent/failed` 和固定去敏错误信息。测试会真实发群消息，配置或发送应由部署操作者显式执行。Grafana provisioning 不提供 Feishu 签名字段，请使用上述原生 API/UI。

## 发送契约

发送文本消息，包含渲染后的标题、消息、告警标签/注释/值、外部地址和通知编号；超长内容截断，并确保编码后的 JSON 不超过 20 KiB。签名使用秒级发送时间，把 `timestamp + "\n" + secret` 作为 HMAC-SHA256 密钥，对空消息求摘要并 Base64 编码，每次发送重新生成。保持服务器时钟同步。算法、20 KB 限制及业务成功码见[飞书官方自定义机器人指南](https://open.feishu.cn/document/client-docs/bot-v3/add-custom-bot)。

保留正常 TLS 证书校验，禁止自动重定向。每次请求总时限为 `timeout_seconds`（1–30 秒，默认 10 秒），包含响应读取。只有 HTTP 2xx、8 KiB 内有效 JSON 且显式 `code: 0` 才算成功；缺失/null/非整数 code、错误业务码均失败，不把 HTTP 200 误记为已送达。业务错误保留数字 code，丢弃 `msg` 和响应正文。沿用现有最多 8 次尝试及 5 秒至 5 分钟退避；错误签名等配置问题也应及时更正，不能靠重试修复。重复投递仍可能发生，飞书不提供此通知编号的去重保证。

验证覆盖本地真实 TLS 接收端的签名、消息上限、成功/错误 ACK、重定向、超时、取消、重试，以及数据库密文、API 脱敏、更新 CAS 和权限。该证据不等于已配置生产机器人或已向真实群发送。
