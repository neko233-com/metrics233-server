import type { Panel } from "../api";

const titles: Record<string, string> = {
  online: "在线玩家", sessions: "累计成功玩家会话", "players-trend": "在线玩家趋势",
  "process-cpu": "进程 CPU · 单核占比", "process-memory": "进程内存", "process-uptime": "进程已运行",
  "process-cpu-trend": "进程 CPU 趋势", "process-memory-trend": "进程内存趋势",
  "process-go-heap": "Go 堆内存", "process-goroutines": "Go 协程",
  "gateway-connections": "当前网关连接", "gateway-queue": "等待发送的消息",
  "gateway-receive": "接收消息 / 秒", "gateway-send": "发送消息 / 秒", "gateway-dropped": "丢弃消息 / 秒",
  "traffic-in": "接收流量 / 秒", "traffic-out": "发送流量 / 秒",
};
const descriptions: Record<string, string> = {
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
