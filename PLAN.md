# metrics233-server 执行计划

更新时间：2026-10-21。

> **本文件是所有后续开发的唯一执行清单。** 每次改动必须先对应到本文件中的一个条目；完成时更新勾选、证据和总进度，并按固定格式汇报：目标、做了什么、接下做什么、总进度百分比、风险、验证。进度明细同步到 [docs/status.md](docs/status.md)。

## 产品目标（固定）

开箱即用的单机监控后台，三合一：

- 替代 Grafana 面板：自有 UI、仪表盘、Explore、告警。
- 数据抓取：Prometheus/OpenMetrics、OTLP/HTTP、MySQL/MariaDB、Redis、PostgreSQL、Linux/业务 exporter。
- 本地时序 + 向量存储：SQLite 默认，ClickHouse 可选；64 维指标窗口向量检索。

默认交付单个原生二进制，`serve` 零配置可用；Docker/ClickHouse 可选。产品 UI 使用 `web/src/ui` 与
`web/src/render`，不追求 Grafana 全量 parity；Grafana 导入/导出仅是可选迁移适配。

## 已验证基线

- Go 1.27、Vite 7、React 19；`serve` 单二进制；Docker 可选。
- 采集：Prometheus/OpenMetrics、OTLP/HTTP、MySQL/MariaDB、Redis、PostgreSQL、服务指标接口。
- 存储：SQLite WAL 默认；ClickHouse 可选；保留期；64 维向量检索。
- 认证：`--auth=token` 与 `--auth=local`；Setup token、会话、Admin/Editor/Viewer、限流与安全头。
- 渲染器：Table、TimeSeries、Stat、Gauge、BarGauge 已默认自有；TimeSeries 可用 `?renderer=compat` 回退。
- 验证：前端单测 104 passed；完整浏览器套件 69 passed / 2 skipped；CI run 39436800402 的 test + docker 全绿。
- CI 策略：仅 `v*` tag 或手动 dispatch 触发昂贵验证；普通 push 不触发。

## 总进度

| 工作流 | 权重 | 完成度 | 主要缺口 |
| --- | ---: | ---: | --- |
| 产品 Shell 与自有 UI | 15% | 95% | 业务接入页、按需加载、焦点竞态修复；可选动效不阻塞生产 |
| 渲染器 | 15% | 95% | Tempo 详情、Loki 实际查询通过；真实 Tempo daemon 未验收 |
| 采集 | 15% | 95% | OTLP/remote-write 严格预检、上限、部分成功与真库采集已验收；协议支持子集有明确契约 |
| 存储与查询 | 15% | 95% | SQLite bucket-last/选序列下推、PromQL 保真上限、真实 ClickHouse 大范围实测；不引入破坏覆盖语义的预聚合 |
| 认证与安全 | 15% | 95% | API namespace、失败限流、代理来源和 HTTPS root 已修复；真实公网配置仍需目标验收 |
| 运维与交付 | 10% | 90% | 正式 runtime、完整数据恢复和可复现归档已验；拆分后的 tag CI 与线上交付进行中 |
| 文档与 agent 契约 | 5% | 95% | 契约与实测同步；持续维护 |
| 性能 | 10% | 90% | Worker/min-max、100k/1m 基准与大范围存储实测；JSON 解析与增量归一化仍有边界 |

**加权总进度：94%**（权重合计 100%，±5 个百分点）。
这是路线图完成估算，不是生产可用率或“所有环境已验收”的声明。每完成一个执行项，更新本表与 `docs/status.md`。

## 执行队列

- [~] **G2 — 游戏运维与账户边界**：账户登录与仅上报凭据分离、可配置 15 天留存、实例虚拟仪表盘、全量地区/SQL 虚拟列表、风险预警线/中文/数字运行时、飞书卡片与 agent HTTP discovery。源码与针对性测试已落地，最终冻结、完整回归、不可变发布与真实目标验收分层记录于 docs/status.md。

状态：`[ ]` 未开始，`[~]` 进行中，`[x]` 完成。按顺序执行；未完成当前项前不并行开启下一项，除非明确标注可并行。

本次生产完善批次允许 **R2b / P1 / B1 / O2 并行**：分别拥有 Tempo 详情、时序模型、业务接入页、Docker 验证文件；
共享构建与最终浏览器回归在合并后统一执行。F1 随统一回归复查。可选动效和工具链迁移不作为生产就绪条件。

后续 **D1 / C1 / A1 / O1 可并行**：分别拥有 store 查询、采集协议、认证边界、发布脚本；共享集成测试与最终镜像
在源码冻结后顺序执行。O1 先交付可复现本地产物与清单，外部 tag/推送仍依授权执行。

- [x] **B1 — 业务指标接入闭环（可并行）**
  - 范围：第一方业务监控入口、真实指标选择、接入示例、业务仪表盘创建；不注入演示数据或凭据。
  - 验收：JSON 推送 → 指标发现 → 创建仪表盘 → 实际图表；Viewer 只读；390/768/1280 无溢出。
  - 证据：Business 2/2 浏览器通过；真实 Docker Viewer 会话隐藏写入按钮并由后端拒绝写入。
- [x] **O2 — Dockerfile_test 深度验证（可并行）**
  - 范围：复用正式 runtime 的测试镜像、非 root/只读根文件系统、角色、采集查询、重启持久化、备份恢复、资源测量。
  - 验收：真实 Linux 容器执行；失败可定位；只清理本次资源；不将容器验证等同公网部署验收。
  - 证据：正式 runtime+browser 两轮、真库 Docker 两轮、Linux vet/race 通过；详见 `docs/docker-validation.md`。
    默认 Compose 与可选 ClickHouse overlay 已做精确配置校验；没有启动用户生产 Compose 实例。

### P0 产品闭环

- [x] **A2 — 固定上报 Token 验证**
  - 范围：确认业务 JSON、两条 remote-write 和两条 OTLP 路由统一使用固定 Bearer Token；SDK 保留现有 `WithToken` 和链式标签/指标 API。
  - 证据：`TestFixedTokenGuardsEveryMetricIngestTransport` 使用真实本地 HTTP 与有效协议载荷，缺少/错误 Token 均 401 且无落库；正确 Token 成功写入。URL 参数/cookie 不能替代 Bearer 头。SDK 全包通过，独立 server binary 可选测试本轮未启用。
  - 边界：公开代理前必须配置固定 `METRICSPANEL_TOKEN`；未配置 Token 的 loopback 开发兼容行为不变，不能将该模式暴露到公网。

- [~] **A3 — 安全分发上报专用凭据与来源策略**
  - 范围：`METRICSPANEL_INGEST_TOKEN` 与管理 Token 分离，仅 token 模式，环境输入、32–8192 非空白 ASCII、独立管理 Token；五条精确 POST ingest 允许，管理/查询/解封/Live/未知路径拒绝。
  - 已验：真实 HTTP 五协议成功、管理拒绝、错误/重复/混合头及 query/cookie 拒绝；来源封禁后专用凭据不能自解封。独立 review 覆盖策略 DB 失败读体/写库前拒绝、辅助观测失败保留原 ACK、partial RW 只计已完成批次、mapped IP/多字段代理链、组织隔离、32 并发计数、16 同版 CAS 及满策略容量事务回滚。
  - 最终浏览器已验：两份独立随机凭据、来源一键策略及冲突刷新；完整套件 86 passed / 2 SQL 环境跳过。发布和线上第二个专用 Token 私有配置仍单独验收。契约见 `docs/reporting-access.md`，不给上报方管理 Token。

- [~] **N1 — 飞书群机器人通知（可与部署代理工作并行）**
  - 范围：在既有联系点/策略/持久化队列中支持 Feishu v2 Webhook、可选签名、凭据加密脱敏、API 角色和 UI 配置。
  - 已验：官方空消息 HMAC、20 KiB 编码体、显式 code=0 ACK；本地真实 TLS firing/resolved/队列重试、非法 ACK/302/超时取消；数据库密文、API 保留/清除/CAS；local Viewer/Editor 不能创建/修改/测试/重试，Admin 可用。相关 Go 全包/vet、CLI 全包、前端 typecheck/build/118 unit 通过。
  - 最终浏览器配置流程通过；版本发布、生产私有配置与真实机器人送达另由主任务验收。不得把 202 入队或 HTTP 200 当作飞书成功。
  - 契约：`docs/feishu-notifications.md`；本轮没有向用户实际 Webhook 发送消息，也不在公开文件保存真实地址/签名。

- [~] **N2 — 告警通知冷却与防抖（与 N1 并行收口）**
  - 范围：规则 `notification_cooldown_seconds` 为 0–604800 秒；旧 API/旧规则默认 0，新建 UI 默认 3600 秒。保持持续 Firing 默认不重复，显式 repeat、for、恢复等待与失败重试分别控制。
  - 已验：成功 ACK 与 sent 同事务持久化，分组/非分组恢复后延迟、跨重启、首次/新实例/新接收者/severity 不受阻；未 ACK 周期提前恢复不发孤立恢复；失败保留原 ID 按退避重试；规则编辑、暂停和联系点停启不清锚点；历史清理后恢复回执保留。同周期提醒取至少 max(CD,repeat)，repeat10s/CD300s 红→绿；CD0 保留 10s，repeat0 不被 CD 唤醒成隐式提醒，静默恢复提醒及正常恢复分别验证。
  - 最新 owning 验证：alerting/server/localauth/model/CLI 全包通过（18.485/34.672/4.625/0.068/5.093 秒），相关七包 vet 与 diff check 通过。真实 UI 字段/默认值/旧值保留已通过最终浏览器；生产规则配置由对应交付任务完成。
  - 契约：`docs/alert-notification-cooldown.md`。CD 是所有成功 firing 的最低间隔，更短 repeat 不能绕过；本地传入调度时间验证不等于线上已连续运行一小时。

- [x] **C0 — 推 + 拉双模式**
  - 范围：拉取抓取与推送接收同时可用，共享存储与角色策略。
  - 验收：Prometheus remote write、OTLP/HTTP、原生 JSON 推送与 scrape 拉取均有测试；Viewer 推送 403。
  - 证据：`POST /api/v1/write` 与 `/prometheus/api/v1/write`（snappy `prompb.WriteRequest`，32 MiB 压缩 /
    64 MiB 解码 / 100,000 样本）；`remote_write_test.go` 覆盖 snappy、原始 protobuf、畸形输入、空 `__name__`；
    角色测试覆盖 Viewer 403 / Editor 允许；文档 `docs/collection-modes.md`。

- [x] **R0 — Table / TimeSeries / Stat / Gauge / BarGauge 自有渲染器**
  - 证据：`web/src/render`，完整套件 69 passed / 2 skipped，commit `404c0d1`。
  - 验收：registry 解析、浏览器契约、390/768/1280、无 `@grafana/ui` 产品引用。

- [x] **S1 — 左侧导航 + 右侧内容 Shell**
  - 范围：可折叠左侧导航、右侧主内容区、顶栏；保留现有路由、权限、深链和移动端行为。
  - 验收：390/768/1280 无横向溢出；键盘可完成导航与折叠；Viewer 看不到写操作；现有浏览器套件全绿。
  - 证据：桌面折叠/展开、localStorage 持久化、键盘 Enter、390 移动抽屉、768/1280 无溢出；完整套件
    69 passed / 2 skipped（1 个既有变量用例首次抖动、重试通过，跟踪为 F1）。

- [x] **R1 — Logs 渲染器**
  - 范围：Loki frames、虚拟化行、标签过滤、搜索、空态/错误态。
  - 验收：10,000 行仍可交互；滚动与过滤有浏览器测试；不新增 `@grafana/ui` 产品引用。
  - 证据：`LogsRenderer` 虚拟化 10k 行、搜索、标签键/值过滤；浏览器测试断言 `data-log-count`、
    实际 DOM 行数 < 60、滚动高度、过滤结果与 1280 无横向溢出；完整套件 70 passed / 2 skipped。

- [x] **R2 — Traces 渲染器**
  - 范围：Tempo 搜索表格优先，trace 详情随后；耗时、状态、深链参数。
  - 验收：搜索与详情浏览器测试；空态/错误态；与后端 frame 契约一致。
  - 证据：`TracesRenderer` 读取 Tempo `traceID/rootServiceName/rootTraceName/startTime/durationMs`；支持搜索、
    按开始时间/耗时排序、可选 status 列与 `?traceId=` 深链；浏览器测试覆盖搜索、双向排序、深链回填与无溢出。
    当时完整套件 72 passed / 2 skipped；后续 Trace 详情已由 R2b 交付。

- [x] **R2b — Trace 详情**
  - 范围：按 traceId 获取 span 树/时间轴；先补后端 Tempo trace 查询，再做详情视图。
  - 验收：真实 Tempo fixture；span 层级、耗时与错误状态；`?traceId=` 深链。
  - 证据：官方固定 SHA fixture、边界/取消 Go 测试、实际 HTTP→Go→UI 浏览器通过；非 live Tempo daemon 验收。

- [x] **R3 — Heatmap / Histogram / Vector 渲染器**
  - 范围：按渲染器矩阵补齐常用分析视图；向量视图读取 64 维字段。
  - 验收：每类一个浏览器契约；大 frame 有分帧或降采样。
  - 证据：`HeatmapRenderer` 画布时间桶/图例/悬浮值；`HistogramRenderer` 归一化柱状；
    `VectorRenderer` 64 维热条与悬浮值；三个浏览器测试分别断言 `data-cell-count`、`data-bar-count`、
    `data-vector-count/dimensions` 与无横向溢出。

- [x] **P1 — Worker 降采样**
  - 范围：LTTB 或 min/max + transferable；与 `frame-budget`/`incremental` 协同。
  - 验收：100k 点基准；峰值与断点保持；首帧预算写入 `docs/performance-baseline.md`。
  - 证据：17 项定向单测、6 项浏览器；最终 binary 的 100k×5 / 1m×1 各三次实测与已知长任务已记录。

### P1 深度与交付

- [x] **F1 — 稳定仪表盘变量解析时序**
  - 现象：`dashboard-variables` 偶发在预览或保存后面板查询中出现未解析的 `$var`。
  - 验收：`npx playwright test dashboard-variables.spec.ts --retries=0` 连续 3 次通过；面板不得在变量未就绪时发查询。
  - 根因/证据：Dialog/AppModal 的双重 rAF autofocus 抢走编辑焦点；改为首次绘制前聚焦。确定性回归与原用例
    连续三轮零重试 12/12；变量解析既有屏障保持不变。

- [x] **D1 — ClickHouse rollup 与大范围查询**
  - 范围：rollup/降采样、块级 min/max、保留期与 TTL 操作。
  - 验收：大范围查询计划与基准；SQLite 回退或明确限制。
  - 证据：SQLite SQL bucket-last、5M扫描预算及原始rate/PromQL回归；1M/30天三次查询约465–473ms、Go分配1.06MB。
    真实ClickHouse `FINAL+argMax` 三次61–81ms；额外block min/max无收益，未添加冗余索引或破坏历史覆盖的物化视图。
    另修复换行label的PromQL正则语义；TTL操作和容量边界见 `docs/storage-query-planning.md`。

- [x] **C1 — 采集深度**
  - 范围：更多 exporter、OTLP histogram 边界、采集失败模式。
  - 验收：协议测试 + 超时/认证/畸形输入测试。
  - 证据：OTLP histogram/optional sum/partial_success/10k展开预算、RW解压前预算及完整预检、缺name零写入、存储失败503；
    pull格式标签身份、4MiB直读限制与HTTP超时/认证/重定向测试。精确支持子集及事务边界见 `docs/collection-modes.md`。

- [x] **A1 — 认证与安全复查**
  - 范围：会话轮换、限流、可信代理、备份/恢复、泄露清单。
  - 验收：复查记录 + 回归测试；文档同步。
  - 证据：真实server namespace匿名拒绝/合法session可读/Viewer不可写；Basic与form共享失败预算、并发/重定向绕过、
    XFF多字段顺序与4096状态上限、显式HTTPS Origin、Setup token race回归。owning全包与vet通过；
    窄审查记录 `docs/security-readiness-review.md`，最终Linux/race与HTTP upstream容器验收单独记录。

- [~] **O1 — 发布收口**
  - 范围：版本升级说明、备份恢复演练、release checklist、tag 验证。
  - 验收：`v*` tag 触发完整 CI 全绿；发布归档可复现。

### 可选评估

- [ ] **V1 — Vite+ 评估**
  - 范围：隔离分支比较 build/lint/test/CI 成本。
  - 验收：书面决策；未证明 parity 前不迁移。
- [ ] **V2 — ReactBits 动效层**
  - 范围：仅装饰性动效；不改布局、焦点、对比度和表单行为。
  - 验收：尊重 `prefers-reduced-motion`；现有验收全绿。

## 完成定义

- 每个执行项必须有：代码、至少一个可执行验收、文档/台账更新、明确证据。
- 不得用源码阅读代替浏览器、Docker 或 CI 证据。
- 产品页面与 renderer 不得新增 `@grafana/ui` 引用；兼容桥只允许存在于 `web/src/grafana`。
- 响应式验收固定覆盖 390/768/1280；交互必须有键盘路径。
- 未完成项不得标记为完成；部分完成保持 `[~]` 并说明剩余。

## 本批已落地（2026-10-20）

| 提交 | 内容 |
| --- | --- |
| `404c0d1` | Stat/Gauge/BarGauge 进入第一方 registry；完整套件 69 passed / 2 skipped |
| `a51c275` | 图表与表格响应式验收（390/768/1280、动态高度、窄屏表格） |
| `ef500a8` | TimeSeries 自有渲染器设为默认，`?renderer=compat` 回退 |
| `e21a7ea` | 进度台账与固定汇报协议 |
| `031a8b9` | Oil UI skill 引用与边界 |
| `dcb2415` | Agent 契约；CI 改为 tag/手动触发 |

## 历史归档

- [x] **通知超时诊断竞态**：客户端/传输层 deadline 先于 request context 时，仍返回固定去敏超时信息；取消和普通网络错误保持通用失败。新增四项确定性用例，保留真实 TLS 超时用例；Windows alerting 测试与 vet 通过，最终 Linux race、不可变发布和生产验收独立记录。

- [~] **飞书卡片与自定义通知模板（2026-10-21）**：默认状态色卡片、文本兼容、主机/资源/评估值/for/恢复/CD、可信工作台详情按钮；复用有界单次模板和持久化快照，Admin-only 示例预览不外发。定向 Go/TLS/权限/ACK/CD、Linux vet/race 两轮 80 项、最终统一 UI 通知浏览器 4 项和业务 3 项已通过。真实 DB 两轮及最终源码 runtime/browser 两轮通过，分层输入与资源证据见 `docs/docker-validation.md`；完整 Docker race target 本轮未重跑。不可变发布和真实群卡片由主任务独立收口，不降低正式资源规则的持续触发时间。契约见 `docs/feishu-notifications.md`。

- [~] **A3 — 上报来源与来源访问限制**：主导航、协议/接收计数/最近时间、Admin 一键拉黑/解除/允许、可选白名单模式。
  五协议拒写、可信代理/伪造头、IPv4-mapped、版本冲突、重启与容量/封禁独立保留已通过；浏览器最终版、独立仅上报 Token 和发布仍单独验收。
  契约见 `docs/reporting-sources.md`。

2026-10-13 之前的 Grafana 全量 parity 计划、资源 ACL、通知队列和身份隔离工程笔记已由 git 历史保存，不再作为
当前路线图。需要兼容性背景时阅读 [docs/grafana-compatibility.md](docs/grafana-compatibility.md)、
[docs/grafana-product-parity.md](docs/grafana-product-parity.md) 和 [docs/agent/decisions.md](docs/agent/decisions.md)。
