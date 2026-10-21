/** Presentation only: never use friendly names as metric/query or source identities. */
export type PresentationLocale = "en" | "zh";
export type MetricPresentation = { metric: string; name: string; description: string; known: boolean };
export type SeriesInput = {
  metric?: string;
  labels?: Readonly<Record<string, string>>;
  /** Already resolved field display name; takes precedence over a legend template. */
  displayName?: string;
  legend?: string;
  fallbackName?: string;
  reference?: string;
  comparison?: boolean;
  /** Native query aggregation, never inferred from arbitrary PromQL. */
  aggregation?: string;
};
export type SeriesPresentation = MetricPresentation & {
  custom: boolean;
  rawName: string;
  rawLabels: Readonly<Record<string, string>>;
};

type Definition = readonly [zh: string, en: string, zhDescription: string, enDescription: string];
// Exact names only. A rate/ratio expression without __name__ must use its own
// query alias; a metric's original counter semantics cannot describe that result.
const METRICS: Readonly<Record<string, Definition>> = {
  metricspanel_uptime_seconds: ["监控服务运行时长", "Monitoring service uptime", "当前监控服务进程的运行秒数；重启后重新计时。", "Seconds since the current monitoring service process started; resets on restart."],
  metricspanel_memory_bytes: ["监控服务 Go 堆内存", "Monitoring service allocated Go heap", "runtime.MemStats.Alloc：当前已分配的 Go 堆对象字节数；随分配和垃圾回收变化，不是进程 RSS 或主机内存。", "runtime.MemStats.Alloc: bytes allocated to Go heap objects; varies with allocation and garbage collection, not process RSS or host memory."],
  metricspanel_goroutines: ["监控服务协程数", "Monitoring service goroutines", "runtime.NumGoroutine()：当前监控服务进程中存在的 Go goroutine 数量；不是主机线程数。", "runtime.NumGoroutine(): current Go goroutines in the monitoring service process; not host threads."],
  metricspanel_http_requests_total: ["监控服务累计 HTTP 请求数", "Monitoring service cumulative HTTP requests", "当前服务实例 HTTP handler 接收的累计请求次数；重启后重新计数。每秒速率由 rate 查询计算，原始值不是速率。", "Cumulative requests received by the current service instance's HTTP handler; resets on restart. A rate query computes requests per second; the original counter is not a rate."],
  up: ["采集目标状态", "Scrape target status", "最近一次采集成功为 1，失败为 0；没有样本不代表成功。", "The last scrape succeeded at 1 or failed at 0; missing samples do not mean success."],
  scrape_duration_seconds: ["采集耗时", "Scrape duration", "一次采集请求消耗的秒数。", "Seconds spent on one scrape."],
  scrape_samples_scraped: ["采集样本数", "Scraped samples", "最近一次采集读取的样本数。", "Number of samples read in the latest scrape."],
  node_boot_time_seconds: ["主机启动时间", "Host boot time", "主机启动的 Unix 时间戳；不是运行时长。", "Unix timestamp when the host booted; this is not an uptime duration."],
  node_time_seconds: ["主机当前时间", "Host current time", "主机当前 Unix 时间戳，单位为秒。", "Current Unix timestamp on the host, in seconds."],
  node_cpu_seconds_total: ["累计 CPU 时间", "Cumulative CPU time", "按 CPU 核心和模式累计的秒数；使用率需由查询计算速率或比例。", "Cumulative seconds by CPU core and mode; utilization requires a rate or ratio query."],
  node_load1: ["主机 1 分钟负载", "Host 1-minute load", "过去 1 分钟平均系统负载；不是 CPU 百分比。", "Average system load over one minute; this is not a CPU percentage."],
  node_load5: ["主机 5 分钟负载", "Host 5-minute load", "过去 5 分钟平均系统负载；不是 CPU 百分比。", "Average system load over five minutes; this is not a CPU percentage."],
  node_load15: ["主机 15 分钟负载", "Host 15-minute load", "过去 15 分钟平均系统负载；不是 CPU 百分比。", "Average system load over fifteen minutes; this is not a CPU percentage."],
  node_memory_MemTotal_bytes: ["主机内存总量", "Host total memory", "主机物理内存总字节数。", "Total physical memory on the host, in bytes."],
  node_memory_MemAvailable_bytes: ["主机可用内存", "Host available memory", "系统估计无需交换即可使用的内存字节数，包含可回收部分。", "Estimated memory available without swapping, including reclaimable memory, in bytes."],
  node_memory_MemFree_bytes: ["主机空闲内存", "Host free memory", "完全空闲的内存字节数；不同于可用内存。", "Completely free memory in bytes; different from available memory."],
  node_memory_SwapTotal_bytes: ["交换空间总量", "Total swap", "主机交换空间总字节数。", "Total host swap space, in bytes."],
  node_memory_SwapFree_bytes: ["可用交换空间", "Free swap", "主机剩余交换空间字节数。", "Free host swap space, in bytes."],
  node_filesystem_size_bytes: ["文件系统总容量", "Filesystem capacity", "该挂载点文件系统的总字节数。", "Total filesystem size for this mount point, in bytes."],
  node_filesystem_avail_bytes: ["文件系统可用空间", "Filesystem available space", "该挂载点普通用户可用的字节数；不同于包含保留空间的 free。", "Bytes available to non-root users on this mount; differs from free space including reserved blocks."],
  node_filesystem_free_bytes: ["文件系统空闲空间", "Filesystem free space", "该挂载点空闲字节数，包含可能只供管理员使用的保留空间。", "Free bytes on this mount, including blocks that may be reserved for administrators."],
  node_filesystem_readonly: ["文件系统只读状态", "Filesystem read-only state", "文件系统只读为 1，可写为 0。", "1 when the filesystem is read-only, 0 when writable."],
  node_filesystem_device_error: ["文件系统采集错误", "Filesystem collection error", "文件系统状态读取失败为 1；失败样本不应当作正常容量。", "1 when filesystem statistics cannot be read; errors must not be treated as healthy capacity."],
  node_filesystem_files: ["文件系统 inode 总量", "Filesystem total inodes", "文件系统提供的 inode 总数。", "Total number of inodes in the filesystem."],
  node_filesystem_files_free: ["文件系统可用 inode", "Filesystem free inodes", "文件系统剩余 inode 数。", "Number of free inodes in the filesystem."],
  node_scrape_collector_success: ["主机采集器状态", "Host collector status", "该主机采集器成功为 1，失败为 0。", "1 when this host collector succeeded, 0 when it failed."],
  node_disk_read_bytes_total: ["累计磁盘读取", "Cumulative disk reads", "设备累计读取字节数；每秒速率由查询计算。", "Cumulative bytes read from the device; per-second throughput requires a rate query."],
  node_disk_written_bytes_total: ["累计磁盘写入", "Cumulative disk writes", "设备累计写入字节数；每秒速率由查询计算。", "Cumulative bytes written to the device; per-second throughput requires a rate query."],
  node_network_receive_bytes_total: ["累计网络接收", "Cumulative network received", "网络设备累计接收字节数；不是当前带宽。", "Cumulative bytes received by the network device; this is not current bandwidth."],
  node_network_transmit_bytes_total: ["累计网络发送", "Cumulative network sent", "网络设备累计发送字节数；不是当前带宽。", "Cumulative bytes sent by the network device; this is not current bandwidth."],
  process_resident_memory_bytes: ["进程常驻内存", "Process resident memory", "当前进程驻留在物理内存中的字节数。", "Bytes of the current process resident in physical memory."],
  process_virtual_memory_bytes: ["进程虚拟内存", "Process virtual memory", "当前进程虚拟地址空间字节数；不是常驻内存。", "Process virtual address space in bytes; this is not resident memory."],
  process_cpu_seconds_total: ["累计进程 CPU 时间", "Cumulative process CPU time", "进程累计消耗的 CPU 秒数；不是 CPU 使用率。", "Cumulative CPU seconds consumed by the process; this is not CPU utilization."],
  process_start_time_seconds: ["进程启动时间", "Process start time", "进程启动的 Unix 时间戳；运行时长需用当前时间减去该值。", "Unix timestamp when the process started; subtract from current time for uptime."],
  process_open_fds: ["进程打开文件数", "Process open file descriptors", "当前进程已打开的文件描述符数量。", "Current number of open file descriptors in the process."],
  go_goroutines: ["Go 协程数", "Go goroutines", "当前存活的 Go goroutine 数量。", "Current number of live Go goroutines."],
  go_memstats_heap_alloc_bytes: ["Go 堆已分配内存", "Go allocated heap", "当前 Go 堆中已分配对象的字节数；不是完整进程内存。", "Bytes allocated to objects on the Go heap; not total process memory."],
  go_memstats_alloc_bytes: ["Go 已分配对象内存", "Go allocated object memory", "当前已分配且尚未释放的 Go 堆对象字节数。", "Bytes allocated to Go heap objects that have not yet been freed."],
  go_memstats_sys_bytes: ["Go 向系统申请的内存", "Go memory obtained from OS", "Go 运行时从操作系统获取的字节数；不是全部业务对象大小。", "Bytes obtained by the Go runtime from the OS; not the size of business objects alone."],
  gateway_connections_active: ["网关当前连接数", "Gateway active connections", "当前 TCP 连接数；不是已认证在线玩家人数。", "Current TCP connections; not the number of authenticated online players."],
  gateway_connections_total: ["网关累计连接数", "Gateway cumulative connections", "累计建立的连接次数；重连会再次计数，不是去重玩家数。", "Cumulative connections established; reconnects count again, not distinct players."],
  gateway_messages_received_total: ["网关累计接收消息", "Gateway cumulative received messages", "累计收到的消息条数；每秒速率由查询计算。", "Cumulative messages received; per-second throughput requires a rate query."],
  gateway_messages_sent_total: ["网关累计发送消息", "Gateway cumulative sent messages", "累计发送的消息条数；每秒速率由查询计算。", "Cumulative messages sent; per-second throughput requires a rate query."],
  gateway_bytes_received_total: ["网关累计接收字节", "Gateway cumulative received bytes", "累计收到的字节数；不是当前带宽。", "Cumulative bytes received; this is not current bandwidth."],
  gateway_bytes_sent_total: ["网关累计发送字节", "Gateway cumulative sent bytes", "累计发送的字节数；不是当前带宽。", "Cumulative bytes sent; this is not current bandwidth."],
  gateway_client_write_queue_depth: ["网关待发送队列", "Gateway pending writes", "客户端写队列当前深度。", "Current depth of the client write queue."],
  gateway_client_write_queue_dropped_total: ["网关累计写队列丢弃", "Gateway cumulative dropped writes", "客户端写队列累计丢弃次数。", "Cumulative client write-queue drops."],
  gateway_backend_reconnects_total: ["网关累计后端重连", "Gateway cumulative backend reconnects", "累计后端重连次数；只在实际上报该指标的实现中可用。", "Cumulative backend reconnects; available only in implementations that report this metric."],
  gateway_backend_dial_errors_total: ["网关累计后端连接失败", "Gateway cumulative backend dial errors", "累计后端建连失败次数。", "Cumulative failures to establish a backend connection."],
  gateway_rpc_relay_failures_total: ["网关累计 RPC 转发失败", "Gateway cumulative RPC relay failures", "累计 RPC 转发失败次数。", "Cumulative failed RPC relays."],
  gateway_rpc_online_routes: ["网关在线路由数", "Gateway online routes", "当前可用的 RPC 在线路由数量；不是玩家人数。", "Current number of online RPC routes; not a player count."],
  game_players_online: ["当前在线玩家", "Online players", "权威在线管理器记录的当前已认证玩家人数；不同于网关连接数。", "Current authenticated players reported by the authoritative online manager; different from gateway connections."],
  game_players_online_by_region: ["各地区在线玩家", "Online players by region", "按地区聚合的当前在线玩家；IP 归属地是大致位置，不是精确所在地。", "Current online players aggregated by region; IP geolocation is approximate, not an exact location."],
  game_player_sessions_started_total: ["累计成功玩家会话", "Cumulative successful player sessions", "累计成功建立的玩家会话次数；不是去重玩家数或当前在线人数。", "Cumulative successful player sessions; not distinct players or current online population."],
  game_sql_calls_total: ["SQL 累计实际执行", "Cumulative SQL executions", "实际驱动执行次数，重试分别计数；不是异步入队次数。", "Actual driver execution attempts, counting retries separately; not asynchronous enqueue operations."],
  game_sql_failures_total: ["SQL 累计执行失败", "Cumulative SQL failures", "实际驱动执行失败次数，按表和操作聚合。", "Actual failed driver executions, aggregated by table and operation."],
  game_sql_rows_affected_total: ["SQL 累计影响行数", "Cumulative SQL affected rows", "成功 Exec 返回的 RowsAffected 累计值；不是入队 Entity 数或持久化保证。", "Cumulative RowsAffected returned by successful Exec calls; not queued entities or a durability guarantee."],
  game_sql_duration_seconds_sum: ["SQL 累计执行耗时", "Cumulative SQL execution duration", "实际驱动执行耗时总秒数；平均耗时需除以对应 count。", "Total seconds spent on actual driver executions; divide by the matching count for a weighted average."],
  game_sql_duration_seconds_count: ["SQL 耗时样本数", "SQL duration samples", "执行耗时样本累计次数，包含成功和失败；用于加权平均。", "Cumulative duration samples including successful and failed executions; used for weighted averages."],
  metrics233_push_failures_total: ["累计上报失败", "Cumulative reporting failures", "业务采样上报失败的累计次数。", "Cumulative failed business-metric pushes."],
  metrics233_last_push_success_seconds: ["最近上报成功时间", "Last successful report time", "最近一次成功上报的 Unix 时间戳；不是距今时长。", "Unix timestamp of the last successful push; not time elapsed since that push."],
  mysql_up: ["MySQL 采集状态", "MySQL collection status", "MySQL 原生采集成功为 1，失败为 0。", "1 when native MySQL collection succeeds, 0 on failure."],
  redis_up: ["Redis 采集状态", "Redis collection status", "Redis 原生采集成功为 1，失败为 0。", "1 when native Redis collection succeeds, 0 on failure."],
  redis_memory_used_bytes: ["Redis 已用内存", "Redis used memory", "Redis 报告的已用内存字节数。", "Used memory reported by Redis, in bytes."],
  redis_connected_clients: ["Redis 客户端连接数", "Redis connected clients", "Redis 当前已连接的客户端数。", "Current number of clients connected to Redis."],
  redis_db_keys: ["Redis 数据库键数", "Redis database keys", "对应 Redis 数据库当前键数量。", "Current number of keys in the Redis database."],
};

const LABEL_NAMES: Readonly<Record<string, readonly [string, string]>> = {
  serverId: ["服号", "Server"], role: ["角色", "Role"], environment: ["环境", "Environment"],
  channel: ["渠道", "Channel"], service: ["服务", "Service"], mountpoint: ["挂载点", "Mount"],
  device: ["设备", "Device"], cpu: ["核心", "Core"], mode: ["模式", "Mode"], table: ["表", "Table"],
  operation: ["操作", "Operation"], country: ["国家/地区", "Country/region"], province: ["省/州", "Province"],
  city: ["城市", "City"], db: ["数据库", "Database"], database: ["数据库", "Database"],
  cmd: ["命令", "Command"], command: ["命令", "Command"], method: ["方法", "Method"],
  status: ["状态", "Status"], code: ["状态码", "Code"], route: ["路由", "Route"],
  quantile: ["分位", "Quantile"], le: ["桶上界", "Bucket upper bound"], direction: ["方向", "Direction"],
  instance: ["实例", "Instance"], job: ["采集任务", "Job"], implementation: ["实现", "Implementation"],
};
const IMPORTANT_LABELS = ["role", "serverId", "environment", "channel", "service", "mountpoint", "device",
  "cpu", "mode", "table", "operation", "country", "province", "city", "db", "database", "cmd", "command",
  "method", "status", "code", "route", "quantile", "le", "direction"];
const DISAMBIGUATION_LABELS = ["instance", "job", "implementation"];

/** Localize a known original metric; never guess from a prefix or an expression. */
export function metricPresentation(metric: string, locale: PresentationLocale): MetricPresentation {
  const definition = Object.hasOwn(METRICS, metric) ? METRICS[metric] : undefined;
  return {
    metric,
    name: definition ? definition[locale === "zh" ? 0 : 1] : metric || (locale === "zh" ? "数值" : "Value"),
    description: definition ? definition[locale === "zh" ? 2 : 3]
      : locale === "zh" ? "暂无该指标的内置说明；请结合原始指标、标签和查询确认含义。"
        : "No built-in definition is available; inspect the original metric, labels and query to confirm its meaning.",
    known: Boolean(definition),
  };
}

function clipped(value: string): string {
  return value.length <= 96 ? value : value.slice(0, 95) + "…";
}
function labelText(key: string, value: string, locale: PresentationLocale): string {
  const label = Object.hasOwn(LABEL_NAMES, key) ? LABEL_NAMES[key][locale === "zh" ? 0 : 1] : key;
  const role = value === "gateway" ? ["网关", "Gateway"] : value === "logic" ? ["逻辑服", "Logic"] : undefined;
  const display = key === "role" && role ? role[locale === "zh" ? 0 : 1] : value;
  return `${label}=${clipped(display)}`;
}
function explicitName(value: string | undefined): string | undefined {
  return value?.trim() && value !== "__auto" ? value : undefined;
}
function rawSelector(metric: string, labels: Readonly<Record<string, string>>, fallback: string): string {
  const pairs = Object.keys(labels).filter(key => key !== "__name__").sort()
    .map(key => `${key}=${JSON.stringify(labels[key])}`);
  return (metric || fallback) + (pairs.length ? `{${pairs.join(",")}}` : "");
}

/**
 * Present an entire panel's series together so hidden instance/job labels are
 * restored only when needed to distinguish real series. Values/labels are never
 * changed. Explicit aliases are preserved, with a suffix only for a collision.
 */
export function seriesPresentations(inputs: readonly SeriesInput[], locale: PresentationLocale): SeriesPresentation[] {
  const usedKeys: Set<string>[] = [];
  const result = inputs.map(input => {
    const rawLabels = Object.fromEntries(Object.entries(input.labels || {}).filter(([, value]) => typeof value === "string"));
    const metric = rawLabels.__name__ || input.metric || "";
    const source = metricPresentation(metric, locale);
    const rate = input.aggregation === "rate";
    const presentation = !rate ? source : {
      ...source,
      name: metric === "metricspanel_http_requests_total"
        ? locale === "zh" ? "监控服务 HTTP 请求速率" : "Monitoring service HTTP request rate"
        : source.name + (locale === "zh" ? "（每秒变化率）" : " (per-second change)"),
      description: locale === "zh"
        ? "当前显示源指标的每秒变化率，不是原始采样值或累计总次数；实际单位和口径请结合来源与查询确认。"
        : "This query returns the source metric's per-second change, not its raw sample or cumulative total. Confirm the unit and meaning with the source and query.",
    };
    const legend = explicitName(input.legend)?.replace(/\{\{\s*([\w]+)\s*\}\}/g,
      (_, key: string) => Object.hasOwn(rawLabels, key) ? rawLabels[key] : "");
    const alias = explicitName(input.displayName) || explicitName(legend);
    const fallback = input.fallbackName && input.fallbackName !== "Value" ? input.fallbackName : undefined;
    let name = alias || (metric ? presentation.name : fallback || presentation.name);
    const keys = new Set<string>();
    if (!alias) {
      const context = IMPORTANT_LABELS.filter(key => Object.hasOwn(rawLabels, key) && rawLabels[key] !== "");
      context.forEach(key => keys.add(key));
      if (context.length) name += " · " + context.map(key => labelText(key, rawLabels[key], locale)).join(" · ");
    }
    if (input.comparison && !/\((?:comparison|比较)\)$/i.test(name)) name += locale === "zh" ? " (比较)" : " (Comparison)";
    usedKeys.push(keys);
    return { ...presentation, name, custom: Boolean(alias), rawName: rawSelector(metric, rawLabels, input.fallbackName || ""), rawLabels };
  });

  // Resolve collisions per group, not against the full panel, and stop as soon
  // as the visible identities differ. No metric suffix heuristic or player IDs.
  const groups = new Map<string, number[]>();
  result.forEach((item, index) => {
    const members = groups.get(item.name);
    if (members) members.push(index); else groups.set(item.name, [index]);
  });
  for (const members of groups.values()) {
    if (members.length < 2) continue;
    const allKeys = new Set(members.flatMap(index => Object.keys(result[index].rawLabels)));
    const keys = [...IMPORTANT_LABELS, ...DISAMBIGUATION_LABELS,
      ...Array.from(allKeys).filter(key => key !== "__name__" && !IMPORTANT_LABELS.includes(key)
        && !DISAMBIGUATION_LABELS.includes(key)).sort()];
    let partitions = [members];
    for (const key of keys) {
      const next: number[][] = [];
      for (const group of partitions) {
        const distinct = new Set(group.map(index => result[index].rawLabels[key]));
        if (group.length > 1 && distinct.size > 1) {
          for (const index of group) {
            if (usedKeys[index].has(key)) continue;
            const value = Object.hasOwn(result[index].rawLabels, key) ? result[index].rawLabels[key] : "∅";
            result[index].name += " · " + labelText(key, value, locale);
            usedKeys[index].add(key);
          }
        }
        const byName = new Map<string, number[]>();
        for (const index of group) {
          const name = result[index].name, indices = byName.get(name);
          if (indices) indices.push(index); else byName.set(name, [index]);
        }
        next.push(...byName.values());
      }
      partitions = next;
      if (partitions.every(group => group.length === 1)) break;
    }
    for (const group of partitions) {
      if (group.length < 2) continue;
      const refs = new Set(group.map(index => inputs[index].reference));
      if (refs.size > 1) for (const index of group) {
        const reference = inputs[index].reference;
        if (reference) result[index].name += ` (${clipped(reference)})`;
      }
    }
  }
  // Also cover colliding clipped values, repeated references and a literal
  // user alias that happens to equal a generated disambiguation suffix.
  const names = new Set<string>();
  const nextSuffix = new Map<string, number>();
  for (const item of result) {
    const base = item.name;
    let suffix = nextSuffix.get(base) || 2;
    while (names.has(item.name)) item.name = `${base} #${suffix++}`;
    nextSuffix.set(base, suffix);
    names.add(item.name);
  }
  return result;
}
