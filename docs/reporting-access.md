# 上报专用 Token 与来源 IP 管理

给业务 SDK 或其他上报方使用 **ingest-only Token**，不要分发管理 Token。二者都通过 `Authorization: Bearer …` 发送，但权限不同。

| 凭据 | 服务端环境变量 | 权限 |
| --- | --- | --- |
| 管理 Token | `METRICSPANEL_TOKEN` | 原有 workspace 管理、读取和上报权限，包括修改来源策略 |
| 上报专用 Token | `METRICSPANEL_INGEST_TOKEN` | 仅 POST 原生 JSON、两条 remote-write 和两条 OTLP 地址；仍受来源 IP 策略约束 |

## 启动配置

上报专用变量只在 `METRICSPANEL_AUTH=token` / `--auth=token` 支持；在 local 模式配置会明确拒绝启动，不绕过用户认证或授权服务。必须同时配置独立的管理 Token（至少 16 字符），专用 Token 为 **32–8192 个可打印 ASCII 非空白字符**，且不能和管理 Token 相同。建议分别生成随机 64 位十六进制值，只保存于权限受控的私有配置。

`METRICSPANEL_INGEST_TOKEN` 没有命令行参数，避免出现在进程参数或 shell 历史。错误、帮助和 CLI schema 只返回字段名/约束，不打印实际值。环境变量不是数据库加密凭据：Linux 部署应使用 root/服务用户可读的私有 EnvironmentFile，不把文件、Token 或真实来源地址提交仓库。变量为空时保留旧配置兼容行为；未配置管理 Token 的 loopback 开发模式不能公开到反向代理。

只把专用值交给上报程序，例如 SDK 使用 `metrics.New(url, metrics.WithToken(privateIngestToken))`。`WithLabels`、Counter/Gauge/Batch 等视图沿用同一 Token，每次 Push 使用它；SDK API 和旧版本客户端无需变化。SDK 示例中的 `METRICS233_TOKEN` 是调用应用自己的环境变量，应装入专用值，和服务器变量名称不必相同。

专用 Token 只允许以下精确地址和方法：

- `POST /api/v1/ingest`
- `POST /api/v1/write`
- `POST /prometheus/api/v1/write`
- `POST /v1/metrics`
- `POST /otlp/v1/metrics`

携带匹配的专用凭据访问查询、来源列表/策略、告警/通知、Live/session、其他方法、未知地址，甚至公共静态/health 地址，都返回 403。未携带凭据的原公共 health/静态契约不变。缺少/错误凭据、错误认证格式和重复/混合 Authorization 返回 401；URL 参数、cookie 不能代替头。管理 Token 保留原权限，也仍能上报。UI 管理会话必须使用管理凭据。

专用 Token 是单 workspace 的共享写凭据，不是每应用独立账号。它不能自助解封 IP 或读取指标，但知晓它的上报者可以从已允许的地址写入有效样本。轮换该值会影响所有使用此值的上报方；没有按 Token 单独吊销或计费身份。HTTPS、网络隔离和协议/容量上限仍需正确配置。

## 来源策略

`GET /api/v1/reporting-sources` 返回成功上报来源与持久化策略的并集；本地账号可读脱敏业务元数据。`PUT /api/v1/reporting-sources/policy` 要求管理 Token 或 local Admin，更新必须带读取的 `version` 和显式 `allowlist_only`，以及可选 `address` / `action`（`allow`、`block`、`reset`）。版本冲突为 409，缺字段/非法地址为 400。

来源使用真实 socket peer；仅当 peer 位于明确配置的 `METRICSPANEL_TRUSTED_PROXIES` CIDR 时，才从所有 `X-Forwarded-For` 字段右往左选择非可信代理的有效 IP。反向代理必须覆盖该头，或把真实 peer 追加在转发链末端。只信任实际代理地址，不要把普通客户端网段列入可信代理。IPv4-mapped IPv6 归一到 IPv4，与封禁记录使用同一键。策略接受单个 IP，不支持来源 CIDR/端口/zone；NAT 后多个程序会共享来源身份。

默认允许持有有效写凭据的未知来源；显式 `block` 始终拒绝。开启 `allowlist_only` 后，只有显式 `allow` 才能写。来源策略在读取业务载荷和写样本之前检查，存储读取失败返回 503 并拒绝写入。策略变更控制后续入场检查；没有撤回已入场请求或跨后端原子取消正在提交批次的能力。

成功观测保留最近 1024 个地址，计数是已完成的请求/存储批次对应样本数；`service/job` 只是可伪造的显示提示，不用作授权身份。独立策略最多 4096 条，观测淘汰不会删除封禁/允许记录；满容量的新策略更新原子失败，不会顺带关闭 allowlist。持久化表属于具体组织的物理控制库，HTTP 参数或 `X-Grafana-Org-Id` 不切换该库。

样本已经写入后，来源观测是辅助记录：观测失败只记固定去敏警告，保留原来的上报 ACK，避免诱发重复写入。remote-write 部分存储失败仍返回 503 和 `X-Accepted-Samples`，来源只累计已完成的样本，不把失败批次当成成功。来源页不是完整访问审计：无认证、被禁、非法/空载荷及辅助记录失败都可能不产生观测条目。

## 验证

真实本地 HTTP 验证专用 Token 五入口、管理拒绝/防自解封、重复头与替代凭据；定向测试还覆盖鉴权优先、策略表失败零读体/零写入、辅助记录失败保留 ACK、部分 remote-write 计数、IPv4-mapped/代理链、跨组织隔离、并发计数/CAS 和满容量回滚。浏览器、不可变产物及线上 Token 配置由交付任务分别验收；本地回归没有读取线上凭据。
