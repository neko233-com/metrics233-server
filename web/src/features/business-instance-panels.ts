import type { Panel } from "../api";

const titles: Record<string, string> = {
	"process-tokio-tasks":"Tokio 活跃任务", "process-tokio-workers":"Tokio 工作线程",
	"process-http-connections":"HTTP 连接", "process-websocket-connections":"WebSocket 连接",
	"process-http-inflight":"处理中 HTTP 请求", "process-http-rps":"HTTP 请求 / 秒", "process-http-errors":"HTTP 服务错误 / 秒",
	"process-sdk-links":"SDK 出站网关连接", "process-sdk-rpc-pending":"SDK 等待回执的 RPC",
  online: "在线玩家", sessions: "累计成功玩家会话", "players-trend": "在线玩家趋势",
  "process-cpu": "进程 CPU · 单核占比", "process-memory": "进程内存", "process-uptime": "进程已运行",
  "process-cpu-trend": "进程 CPU 趋势", "process-memory-trend": "进程内存趋势",
  "process-go-heap": "Go 堆内存", "process-goroutines": "Go 协程",
  "gateway-connections": "当前网关连接", "gateway-queue": "等待发送的消息",
  "gateway-receive": "接收消息 / 秒", "gateway-send": "发送消息 / 秒", "gateway-dropped": "丢弃消息 / 秒",
  "traffic-in": "接收流量 / 秒", "traffic-out": "发送流量 / 秒",
};
const descriptions: Record<string, string> = {
	"process-tokio-tasks":"当前 Tokio 运行时的实际活跃任务数。", "process-tokio-workers":"Tokio 工作线程数量，与任务数分别统计。",
	"process-http-connections":"HTTP 服务持有的入站连接，升级后的 WebSocket 单独计数；HTTP/2 stream 计为请求而非新连接。",
	"process-websocket-connections":"实际入站 WebSocket，包括客户端和服务 Link；SDK 出站连接单列。",
	"process-http-inflight":"当前执行中的请求处理器，不包括空闲连接。", "process-http-rps":"每秒完成的 HTTP 处理器调用，含升级握手。",
	"process-http-errors":"HTTP 500–599 响应每秒速率。", "process-sdk-links":"SDK 到网关的实际持续连接。", "process-sdk-rpc-pending":"当前等待权威回执的有界 RPC 请求数量。",
  "process-cpu": "100% 表示用满一个 CPU 核心，多核并行时可以超过 100%。这里只计算当前游戏服或网关进程。",
  "process-memory": "当前服务进程实际占用的物理内存，区别于上方整机内存。",
  "process-uptime": "自本次进程启动起的运行时长，重启后重新计算。",
  "process-cpu-trend": "最近5分钟的 CPU 时间增长率；100% 代表用满一个核心。",
  "process-memory-trend": "当前服务进程实际占用的物理内存随时间的变化。",
  "process-go-heap": "Go 对象当前分配的堆内存，不包括线程栈和其他进程内存。",
  "process-goroutines": "当前服务进程中的 Go 协程数。",
  "gateway-connections": "当前保持连接的客户端数量。", "gateway-queue": "客户端发送队列中等待处理的消息。",
  "gateway-receive": "实际收到的消息数，按最近5分钟计算每秒速率。",
  "gateway-send": "实际发出的消息数，按最近5分钟计算每秒速率。",
  "gateway-dropped": "客户端写队列丢弃消息的每秒速率，按最近5分钟计算。",
  "traffic-in": "实际接收流量，按最近5分钟计算每秒速率。", "traffic-out": "实际发送流量，按最近5分钟计算每秒速率。",
};

export function localizeInstancePanel(panel: Panel, locale: string): Panel {
  if (locale !== "zh" || !titles[panel.id]) return panel;
  const title = titles[panel.id];
  return { ...panel, title, config: panel.config ? { ...panel.config, description: descriptions[panel.id] || panel.config.description,
    targets: panel.config.targets?.map(target => target.legendFormat === panel.title ? { ...target, legendFormat: title } : target) } : panel.config };
}

export function instancePanelGroups(panels: Panel[], builtin: boolean) {
  if (!builtin) return [{ id: "custom", panels }];
  return [
    { id: "process", panels: panels.filter(panel => panel.id.startsWith("process-")) },
    { id: "business", panels: panels.filter(panel => !panel.id.startsWith("process-")) },
  ].filter(group => group.panels.length);
}
