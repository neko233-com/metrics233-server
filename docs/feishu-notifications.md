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
  "feishu_format": "interactive",
  "url": "https://open.feishu.cn/open-apis/bot/v2/hook/synthetic-example-only",
  "signing_secret": "synthetic-example-only",
  "timeout_seconds": 10
}
```

更新必须携带最近读取的 `version`；冲突返回 409。已有 Feishu 联系点 `url` 为空时保留已加密的地址，省略 `signing_secret` 保留签名。`clear_secrets: ["signing_secret"]` 或显式 `signing_secret: ""` 删除签名；请同步飞书侧签名设置。切换类型需提供新类型的完整地址。创建和清除地址后没有有效目标时拒绝保存。

联系点保存、更新、删除、测试和通知重试在 local 模式下要求 Admin；Viewer/Editor 可读脱敏配置，不能更改或发送测试。token 模式下使用管理 `METRICSPANEL_TOKEN`；专用 `METRICSPANEL_INGEST_TOKEN` 只能上报指标，不能读写或测试联系点。

`POST /api/v1/alerting/contact-points/{uid}/test` 返回 202 表示**进入队列**，不表示飞书已收到。通过 `GET /api/v1/alerting/notifications?contact_point_uid={uid}` 查看 `pending/sent/failed` 和固定去敏错误信息。测试会真实发群消息，配置或发送应由部署操作者显式执行。Grafana provisioning 不提供 Feishu 签名字段，请使用上述原生 API/UI。

## 发送契约

`feishu_format` 省略或为 `interactive` 时发送告警卡片；`text` 显式保留文本兼容格式。旧联系点不需要迁移数据库即可采用默认卡片。卡片使用飞书支持的经典卡片结构、`plain_text` 文本与跳转按钮：critical 触发为红色、warning/info 触发为橙色、恢复为绿色。分组最多展开 6 项资源，包含主机、资源、中文及原始严重级别、规则评估值、开始/恢复时间、本轮持续时间和创建通知时的 `for` / `keep_firing_for` / CD 配置。剩余资源通过计数提示，在告警页面查看。

默认优先使用规则注释 `title` / `summary`、`threshold` / `condition`、`description` 和显式 `unit`。不解析任意 PromQL 推测阈值或单位；没有阈值注释时显示“满足规则条件”，不默认展开表达式、`managed_by`、`validation_id` 或全部标签。规则评估值未必是原始资源使用率，尤其 `BASE * (BASE > bool T)` 恢复时产生的 0 仅表示条件为假；CPU/内存/磁盘恢复且评估值为 0 时会明确说明，不冒充资源实际使用率为 0。`scope=acceptance` 的整组临时验收始终明确标记“告警链路验收，非资源故障”，时间窗关闭也不冒充实际利用率为 0。

超长内容按 UTF-8 边界截断，限制编码后的完整 JSON（含签名）不超过 20 KiB。签名使用秒级发送时间，把 `timestamp + "\n" + secret` 作为 HMAC-SHA256 密钥，对空消息求摘要并 Base64 编码，每次发送重新生成。保持服务器时钟同步。签名、卡片、20 KB 限制及业务成功码见[飞书官方自定义机器人指南](https://open.feishu.cn/document/client-docs/bot-v3/add-custom-bot)。

保留正常 TLS 证书校验，禁止自动重定向。每次请求总时限为 `timeout_seconds`（1–30 秒，默认 10 秒），包含响应读取。只有 HTTP 2xx、8 KiB 内有效 JSON 且显式 `code: 0` 才算成功；缺失/null/非整数 code、错误业务码均失败，不把 HTTP 200 误记为已送达。业务错误保留数字 code，丢弃 `msg` 和响应正文。沿用现有最多 8 次尝试及 5 秒至 5 分钟退避；错误签名等配置问题也应及时更正，不能靠重试修复。重复投递仍可能发生，飞书不提供此通知编号的去重保证。

请求、HTTP 客户端或网络传输层任一 deadline 到期，均记录固定的 `Webhook request timed out`；取消和其他传输错误记录 `Webhook request failed`。不依赖多个计时器的完成先后，也不保存可能包含凭据的底层错误。

验证覆盖本地真实 TLS 接收端的签名、消息上限、成功/错误 ACK、重定向、超时、取消、重试，以及数据库密文、API 脱敏、更新 CAS 和权限。该证据不等于已配置生产机器人或已向真实群发送。

## 自定义模板与无发送预览

Admin 可在联系点填写 `title_template` / `message_template`，也可用 `{{ template "name" . }}` 复用已有通知模板。文本渲染只执行一次，复用原有 2 秒上下文、10,000 步、128 KiB 输出及模板源/深度预算；这是进程内有界渲染，不是独立子进程沙箱。标签、值和注释不能被再次解析为模板，也不能提供任意卡片 JSON。卡片的状态、计时和资源值独立保留，自定义标题/正文不会改变告警真值、持续时间、CD、分组或 ACK 语义。

常用占位符：

| 上下文 | 字段 |
| --- | --- |
| 整个通知 | `.Status`、`.Receiver`、`.CommonLabels.instance`、`.Alerts` |
| `{{ range .Alerts }}` 内 | `.Labels.resource`、`.Labels.severity`、`.Annotations.title`、`.Values.A` |
| 每条告警冻结的规则元数据 | `.Rule.Title`、`.Rule.ForSeconds`、`.Rule.KeepFiringForSeconds`、`.Rule.NotificationCooldownSeconds` |
| 显式按需展示表达式 | `.Rule.Expr`；默认卡片不会输出 |

旧队列记录没有规则元数据时显示“旧记录未保存”，不把当前可变规则配置冒充历史配置。模板定义、已渲染标题/正文继续使用已有持久化快照，重试或重启不会重新读取新模板内容。

文本只进入 JSON 编码的 `plain_text` 字段；提及标签的尖括号、`@all` 和文本中的 HTTP/WWW 链接前缀被中和。文本兼容格式也应用同样保护，不触发群提及。唯一可点击按钮来自服务端配置的 HTTPS `root-url`（无用户信息/查询参数/片段），指向该工作台的 `/#alerts`，不读取标签、注释或 `generatorURL`。浏览器预览将按钮展示为不可跳转的视觉元素。

`POST /api/v1/alerting/contact-points/preview` 仅 Admin 可调用：

```json
{
  "feishu_format": "interactive",
  "status": "firing",
  "title_template": "{{ .Receiver }} · {{ .Status }}",
  "message_template": "{{ range .Alerts }}{{ .Labels.resource }}={{ .Values.A }}; {{ end }}"
}
```

四项均可省略；默认卡片和 `firing`。响应为 `{format, message, bytes, sent:false}`，`message` 是不含签名/地址凭据的飞书消息 JSON。使用固定示例主机的 CPU、内存、磁盘数据，支持切换 `resolved`，不会保存联系点、评估规则、入队或外发。请求拒绝 URL、secret 或任意 data 字段，避免把预览变成另一个发送接口。模板语法、状态或格式错误返回 400。现有正式资源规则的 10 分钟/5 分钟持续条件不会因预览或卡片格式改变。
