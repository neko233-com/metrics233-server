# metrics233-server 执行计划

- [x] 2026-10-26 移动侧栏遮罩：0.2.29完整tag CI通过Go race和Docker，但最后侧栏用例102通过/2跳过/1失败（重试仍超时）。Linux本地原版五轮复现1失败；新增实际命中区断言在原版立即失败（遮罩x=0，抽屉右边250）。共享移动导航宽度并将遮罩限制到抽屉外，保留正常点击/键盘断言；候选Windows连续5轮、Linux10轮零重试通过。固定17791c5的v0.2.30已公开发布并部署线上，完整tag CI全绿：测试28分钟、Docker10分50秒，托管浏览器103通过/2外部SQL跳过且无重试。最终Windows ZIP完整浏览器103通过，Linux ZIP移动侧栏再10轮与正式runtime/browser两轮通过；七产物匿名下载hash一致，Edge真实导航及原交互验收通过。
- [x] 2026-10-25 GitHub Dark 配色收敛：侧栏/顶栏 #010409，页面 #0d1117、卡片 #161b22、控件 #21262d；灰色选中背景、珊瑚色当前位置、蓝色链接/焦点与绿色主按钮分工明确。表单、弹窗、表格和一等 UI theme 共用语义变量；待触发告警保持黄色。图表/仪表阈值色独立。固定 ebfd82c 的0.2.28已线上发布，Edge真实七实例、筛选搜索、备注恢复、逐网卡流量与卡片/canvas保留通过；169单测/build、101完整浏览器加3真实SQL浏览器、实际Linux归档两轮、两次五平台可复现构建和完整tag CI均通过，七产物匿名下载hash一致。既有Grafana兼容行为保留，各层边界见 docs/status.md。
- [x] 主机默认视图细化：首次选择新鲜且有实际流量的网卡，保留显式选择；仪表按内容区域宽度自适应。完整 server tests/vet、174 项前端单测/build、103 项完整无重试浏览器及2项定向浏览器、实际Linux归档两轮受限runtime/browser和五平台可复现构建通过。0.2.29的托管侧栏失败保留；最终0.2.30完整tag CI、公开发行及匿名下载验收通过。Edge真实七卡片、搜索分类、备注保存/搜索/恢复、逐网卡收发和刷新节点保持通过，1512/1100/768/390分别三/二/一/一列且无溢出。当前/历史Grafana兼容说明已分开，UI与渲染器风格独立。

- [x] 2026-10-25 快速接入与基础设施：真实协议探测、只读预览、带凭据快照证明的原子应用与幂等回执；默认仪表盘和持续/恢复/1h CD 告警，已有采集器复用凭据，固定本机 Exporter 发现。GitHub Dark 控制台与渲染器配色独立，大仪表和移动单列保留自定义布局。模型/HTTP/事务/权限/CLI、完整 Go tests/vet、169 项前端单测/build、6 项布局浏览器通过；修正持续告警后四真库/八协议流程连续两轮通过，最终源码运行容器含 Browser 再通过两轮。固定源码1da41fe的0.2.26已发布；Edge真实七实例、筛选搜索、备注、逐网卡流量、200px仪表和390px无横向溢出通过，刷新保留实际DOM/选择。各层源码与生产边界见 docs/status.md。

- [x] 2026-10-23 实例卡片与网络：GitHub Dark 控制台、全局搜索/分类/环境筛选、独立版本自由备注、Go/Rust 业务字节兼容与逐网卡采集；可视化配色独立，修复跨查询曲线同色。固定源码87258d9的0.2.24完整worker、169项前端单测/build、依赖审计和部署439通过；LAN采集器升级后实际网卡上报通过，线上exporter启用netdev。Edge生产七实例、分类/环境/排序、中文备注保存搜索与恢复、独立业务/网卡流量及网卡切换通过；刷新保留卡片/canvas节点和输入选择，390px无横向溢出。Go/Rust生产端新增别名已推送，实际生产读取仍兼容既有计数器；本条不声称重发游戏二进制或新玩家登录验收。

- [x] 发布资源恢复与 ANN 搜索正确性：页面异常保留导航与手动刷新入口；修复仍有候选时提前退出及错误剪枝。桥接反例先失败后通过，保留原召回断言，30 次连续召回与向量包回归、前端恢复用例/类型检查通过；源码28b6c72的0.2.22完整 worker、正式部署及 Edge 导航验收通过，详见 docs/status.md。

### 2026-10-23 — Bounded slow SQL diagnostics and transport metrics

Native slow SQL counters now feed an exact-instance read API with redacted template variants, bounded Levenshtein groups and table summaries. SDK identities have process/HTTP/WebSocket/RPC panels and do not claim player populations. Targeted real HTTP/auth/instance-isolation/SQL grouping tests, producer lifecycle/TLS ACK and TypeScript checks passed. Fixed-source 0.2.22 full worker and deployment succeeded; Edge verified actual runtime series, SDK panels and the honest empty slow-template window. Populated grouping remains attributed to real local HTTP fixtures, with no injected production SQL. See docs/slow-sql.md and docs/status.md.


更新时间：2026-10-26。

- [x] **实例目录与资源界面重设计**：统一环境分组列表、紧凑搜索/排序、保留返回筛选；实例页突出整机仪表、服务进程和业务指标，低频设置与历史折叠，一处切换大面板。容量改为十进制 MB/GB；完整显示时间统一浏览器时区与格式，顶部独立时钟；当前值移除数据点时间，原始信息仍可展开，过期源样本不冒充当前值。隔离候选167项前端单测/build及业务/主机 Go 测试/vet通过；固定源码548ab439的私有0.2.21完整worker、归档校验和部署成功，Edge实际六实例、内网整机资源、最近60分钟趋势/SQL多排序与桌面/移动视口验收通过。

- [x] **上报服务器仪表盘与多重排序**：同页展示明确关联的整机 CPU/内存/磁盘、实例进程资源和业务指标；继承模板与独立面板保留。SQL/地区支持完整结果集多列排序、优先级编辑和冻结排序的分页游标。环境/重启隔离、绑定CAS及权限、完整 SQL 集合取前10测试与163项前端单测/build、Edge示例渲染通过；私有0.2.19完整worker与部署成功；六实例真实主机与进程查询、Edge内网页和正式服完整84项取前10多重排序已验收。

- [x] **压缩时序与内置向量存储**：控制元数据与时序分离，内置 XOR/Zstd TSDB、Badger/HNSW 向量和独立 SQLite 配置。百万点压缩与崩溃恢复、离线重启校验、Docker恢复和分层race通过；私有0.2.17已完成真实16661384点迁移并恢复新数据接收，初始活动存储约42MB、旧控制库588MB，备份另计。飞书24次状态转换、触发/恢复回执与1小时CD、仅上报token隔离通过；不把样本测试当作通用容量承诺。详见 docs/embedded-storage.md 和 docs/status.md。

- [x] **主机卡片可读性补强**：直接说明 CPU 忙碌程度、可用内存、磁盘剩余容量；从实际告警条件图读取阈值和持续时间，折叠技术口径。160项前端单测/build、告警包与Edge示例渲染通过，私有0.2.19已发布，真实页面与单次签名飞书卡片HTTP200回执通过。

- [x] **主机资源与运维界面**：在现有出站采集器中增加真实 CPU、可用内存与持久磁盘采集，区分缺测和零值；提供主机资源仪表盘、指针式风险仪表、最近60分钟趋势与紧凑运维交互。压缩飞书通知为可读的资源卡片，保留模板与注入防护。内网主机配置持续阈值、恢复确认及1小时冷却；以固定源码发布、实际采集与飞书触发/恢复回执验收，完成后记录具体证据。

- [x] **原生面板与主机管理**：分类柱图、饼图、状态时间线/历史，直方图口径、大数值布局、侧栏三组及无千位分隔显示已实现。Admin SSH 安装、独立出站采集器、加密凭据、版本冲突、暂停/撤销和 MySQL 18 面板完成验收。155 项前端单测、owning Go 包/vet、嵌入构建通过；末次语言/ID 修正另有定向测试和构建。Edge 实测 11 种面板、240 分类虚拟列表及真实 MySQL 数据；Docker SQLite/ClickHouse 全链路 201.59 秒通过并清理资源。生产0.2.12/0.2.13经固定源码完整worker门槛发布，修正真实Mac zsh/sudo票据问题。实际Darwin采集器每批467样本，暂停2/2与恢复3/3通过；凭据与只读上报权限、运行身份和备份单独核对。后续纯文档提交不改变已部署运行源码。见 `docs/managed-hosts.md`、`docs/native-panel-types.md`。

- [x] **面板可读性**：中文/英文指标名与口径、服务器维度消歧、按需原始标签详情、数字运行时长和数据点时间；
  空值与真实零分开、双轴单位独立、全部序列配色、PromQL 单位隔离及亚秒耗时精度。139项前端单测及嵌入构建通过；
  最终9项浏览器回归无重试通过（23.6秒）。首轮完整套件及后续定向修正保留各自归属，线上发布独立验收。
  契约见 `docs/panel-readability.md`；按用户要求不执行游戏测试账号登录，不能据此声称非零玩家链路已验。
- [x] **自监控默认名称补齐**：明确 Go 堆分配内存、协程、累计 HTTP 请求及派生每秒速率的区别；
  最终141项前端单测、类型/嵌入构建和9项无重试浏览器（29.4秒）通过，含默认概览真实图例；生产升级独立记录。

> **本文件是所有后续开发的唯一执行清单。** 每次改动必须先对应到本文件中的一个条目；完成时更新勾选、证据和总进度，并按固定格式汇报：目标、做了什么、接下做什么、总进度百分比、风险、验证。进度明细同步到 [docs/status.md](docs/status.md)。

## 产品目标（固定）

开箱即用的单机监控后台，三合一：

- 替代 Grafana 面板：自有 UI、仪表盘、Explore、告警。
- 数据抓取：Prometheus/OpenMetrics、OTLP/HTTP、MySQL/MariaDB、Redis、PostgreSQL、Linux/业务 exporter。
- 本地时序 + 向量存储：XOR/Zstd TSDB 与 Badger/HNSW 默认，SQLite 独立保存配置；ClickHouse 可选，支持64维指标窗口。

默认交付单个原生二进制，`serve` 零配置可用；Docker/ClickHouse 可选。产品 UI 使用 `web/src/ui` 与
`web/src/render`，不追求 Grafana 全量 parity；Grafana 导入/导出仅是可选迁移适配。

## 已验证基线

- Go 1.27、Vite 7、React 19；`serve` 单二进制；Docker 可选。
- 采集：Prometheus/OpenMetrics、OTLP/HTTP、MySQL/MariaDB、Redis、PostgreSQL、服务指标接口。
- 存储：内置压缩 TSDB/向量库默认，SQLite 控制元数据；显式 SQLite 指标兼容与 ClickHouse 可选，默认15天保留。
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
| 运维与交付 | 10% | 100% | v0.2.30完整tag CI、五平台可复现归档、匿名下载、备份恢复、固定原生发布与Edge已分层验收 |
| 文档与 agent 契约 | 5% | 95% | 契约与实测同步；持续维护 |
| 性能 | 10% | 95% | Worker/min-max、100k/1m 与正式runtime持续HTTP/PromQL实测；JSON 解析、增量归一化及更长运行仍有边界 |

**加权总进度：95%**（权重合计 100%，±5 个百分点）。
这是路线图完成估算，不是生产可用率或“所有环境已验收”的声明。每完成一个执行项，更新本表与 `docs/status.md`。

## 执行队列

- [x] **G2 — 游戏运维与账户边界**：账户登录与仅上报凭据分离、可配置15天留存、实例虚拟仪表盘、全量地区/SQL虚拟列表、风险预警线/中文/数字运行时、飞书卡片与agent HTTP discovery已分层验收。0.2.17原生存储/通知、0.2.19实际实例资源和SQL排序、后续0.2.30完整发布/Edge均有独立回执，见 docs/status.md。用户明确免除游戏测试账号登录，不据此声称非零玩家生命周期已验。

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

- [x] **A3 — 安全分发上报专用凭据与来源策略**
  - 范围：`METRICSPANEL_INGEST_TOKEN` 与管理账号/Token分离，支持token和local模式；环境输入、32–8192非空白ASCII。五条精确POST ingest允许，管理/查询/解封/Live/未知路径拒绝，来源策略仍生效。
  - 已验：真实 HTTP 五协议成功、管理拒绝、错误/重复/混合头及 query/cookie 拒绝；来源封禁后专用凭据不能自解封。独立 review 覆盖策略 DB 失败读体/写库前拒绝、辅助观测失败保留原 ACK、partial RW 只计已完成批次、mapped IP/多字段代理链、组织隔离、32 并发计数、16 同版 CAS 及满策略容量事务回滚。
  - 最终浏览器已验：两份独立随机凭据、来源一键策略及冲突刷新；早期完整套件86通过/2 SQL环境跳过。后续原生0.2.17实际401/403隔离与0.2.30账号初始化/匿名401/仅上报查询403官方healthcheck通过，私有配置回执独立保存。契约见 `docs/reporting-access.md`，不给上报方管理Token。

- [x] **N1 — 飞书群机器人通知（可与部署代理工作并行）**
  - 范围：在既有联系点/策略/持久化队列中支持 Feishu v2 Webhook、可选签名、凭据加密脱敏、API 角色和 UI 配置。
  - 已验：官方空消息 HMAC、20 KiB 编码体、显式 code=0 ACK；本地真实 TLS firing/resolved/队列重试、非法 ACK/302/超时取消；数据库密文、API 保留/清除/CAS；local Viewer/Editor 不能创建/修改/测试/重试，Admin 可用。相关 Go 全包/vet、CLI 全包、前端 typecheck/build/118 unit 通过。
  - 浏览器配置流程通过；后续固定版本已发布并完成真实签名机器人firing/resolved ACK及可读卡片验收，见 docs/status.md 的原生通知与卡片回执。不得把202入队或只有HTTP 200当作飞书成功。
  - 契约：`docs/feishu-notifications.md`。早期本地阶段未外发，后续真实送达有独立回执；公开文件不保存真实地址/签名，本次计划核对不补发消息。

- [x] **N2 — 告警通知冷却与防抖（与 N1 并行收口）**
  - 范围：规则 `notification_cooldown_seconds` 为 0–604800 秒；旧 API/旧规则默认 0，新建 UI 默认 3600 秒。保持持续 Firing 默认不重复，显式 repeat、for、恢复等待与失败重试分别控制。
  - 已验：成功 ACK 与 sent 同事务持久化，分组/非分组恢复后延迟、跨重启、首次/新实例/新接收者/severity 不受阻；未 ACK 周期提前恢复不发孤立恢复；失败保留原 ID 按退避重试；规则编辑、暂停和联系点停启不清锚点；历史清理后恢复回执保留。同周期提醒取至少 max(CD,repeat)，repeat10s/CD300s 红→绿；CD0 保留 10s，repeat0 不被 CD 唤醒成隐式提醒，静默恢复提醒及正常恢复分别验证。
  - Owning全包/vet及真实UI字段/默认值/旧值保留通过；后续实际CPU/内存/磁盘持续/恢复/1h CD规则完成配置。原生引擎实时时钟两轮共24次转换、一次触发与一次恢复ACK、第二周期冷却抑制和自有规则清理通过，归属与早期局部测试分开记录。
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

- [~] **R2c — 真实 Tempo 服务接入闭环**
  - 范围：固定官方 Tempo 发行镜像；真实 OTLP 上报、TraceQL 搜索、trace-ID 详情、层级/耗时/错误状态与仪表盘浏览器，不替换已有 fixture 回归。
  - 验收：隔离的真实服务与监控 HTTP API；实际搜索后打开详情、深链/重载、缺失 trace 和响应式界面；普通账号边界保留。测试仅绑定 loopback，清理自有资源，不连接生产或伪造生产 trace。
  - 当前：真实API/浏览器、Edge错误详情及既有负面边界通过。冷拉取stdout分流后完整CI的Go race、103浏览器与Docker两轮通过，但旧随机夹具撞到前导0长度假设，保留失败归属。最终15b917a固定验证完整/短ID及深链，本地两轮24.688秒、托管专项两轮21.222秒通过；最终组合提交完整复核待收口，见 docs/tempo-live-validation.md。

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

- [x] **P2 — 默认压缩存储持续运行实测**
  - 范围：正式runtime的1CPU/256MiB配置，4000序列、500样本/秒与并发PromQL；读取实际cgroup峰值/CPU及完整活动目录占用，重启核对精确样本数与值。
  - 验收：隔离自有容器/卷，持续10分钟；ACK、查询响应和重启数据一致，记录百分位与环境，不把单次测量当通用SLO。可与R2c托管CI等待并行，各自独立端口和资源；最终台账统一收口。
  - 证据：实际0.2.30 Linux归档、正式runtime限制通过616.148秒；304000样本与伴随写入的600次查询成功（单查询客户端），重启后精确读回。cgroup峰值87.6MB、重启前完整活动数据5.45MB，写入/查询p95为17.534/13.223ms，无OOM/CPU节流；见 docs/embedded-sustained-profile.md。

### P1 深度与交付

- [~] **F2 — 暂停刷新后的当前采样失效**
  - 现象：当前卡片只在重新渲染时计算采样有效期，暂停刷新或后台挂起后可能继续显示旧 CPU、内存和流量。
  - 范围：按采样截止时间唤醒，前台恢复时重新判断；不每秒重绘整个仪表盘，不删除历史趋势，不重建卡片/图表或覆盖备注草稿。
  - 验收：先保留确定性失败，再验证到期隐藏与新采样恢复、DOM/草稿/网卡选择保持；针对性浏览器、Edge 和实际发布分开记录。可与 R2c 的独立完整 CI 等待并行。

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

- [x] **O1 — 发布收口**
  - 范围：版本升级说明、备份恢复演练、release checklist、tag 验证。
  - 验收：`v*` tag 触发完整 CI 全绿；发布归档可复现。
  - 2026-10-25：v0.2.27及v0.2.28各自的完整tag CI全绿；最终ebfd82c五平台两次构建的七产物完全一致，独立ZIP/架构/来源/二进制校验和七文件真实匿名下载通过。Windows归档完整101项通过、2外部SQL跳过由独立真实SQL三项补验；Linux归档两轮受限runtime/browser、四真库与生产0.2.28/Edge分别通过。初版0.2.26的14个浏览器失败和取消test job保留原归属，未改标签或削弱断言。完整证据见 docs/status.md。

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

- [x] **飞书卡片与自定义通知模板（2026-10-21局部阶段，后续已收口）**：默认状态色卡片、文本兼容、主机/资源/评估值/for/恢复/CD、可信工作台详情按钮；有界单次模板与Admin-only不外发预览。早期定向Go/TLS/权限/ACK/CD、两轮Linux vet/race、通知/业务浏览器及真库/runtime证据保持原归属；后续不可变发布、真实签名触发/恢复ACK与可读卡片回执已收口，见 docs/status.md。正式规则持续时间未为验收而降低。契约见 `docs/feishu-notifications.md`。

- [x] **A3 — 上报来源与来源访问限制（早期局部阶段，后续已收口）**：主导航、协议/接收计数/最近时间、Admin一键拉黑/解除/允许、可选白名单模式。
  五协议拒写、可信代理/伪造头、IPv4-mapped、版本冲突、重启与容量/封禁独立保留通过；后续浏览器、独立仅上报Token和正式发布回执见上方A3及 docs/status.md。
  契约见 `docs/reporting-sources.md`。

2026-10-13 之前的 Grafana 全量 parity 计划、资源 ACL、通知队列和身份隔离工程笔记已由 git 历史保存，不再作为
当前路线图。需要兼容性背景时阅读 [docs/grafana-compatibility.md](docs/grafana-compatibility.md)、
[docs/grafana-product-parity.md](docs/grafana-product-parity.md) 和 [docs/agent/decisions.md](docs/agent/decisions.md)。
