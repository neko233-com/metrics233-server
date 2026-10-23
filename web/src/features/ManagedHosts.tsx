import { formatTimestamp } from "../display-format";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { Monitor, MoreHorizontal, Pencil, Plus, RefreshCw, ShieldCheck, ArrowUpRight } from "lucide-react";
import { api, message, timeAgo, type Target } from "../api";
import { getLocale } from "../i18n";
import { Alert, AppModal, Button, Field, Input } from "../ui";
import { useSession } from "../ui/session";
import "./ManagedHosts.css";
import { HostResources, type HostResourcesData, type HostAlertRule } from "./HostResources";

const text = (en: string, zh: string) => (getLocale() === "zh" ? zh : en);
type Host = {
  id: string;
  name: string;
  address: string;
  port: number;
  user: string;
  host_key: string;
  sudo: boolean;
  enabled: boolean;
  revoked?: boolean;
  version: number;
  target: Target;
  secure_fields?: Record<string, boolean>;
  last_seen: number;
  agent_version: string;
  agent_state: string;
  agent_os: string;
  agent_arch: string;
  applied_version: number;
  samples: number;
  collect_host?: boolean;
  resources?: HostResourcesData;
};
type Draft = {
  host: Host;
  password: string;
  privateKey: string;
  collectorPassword: string;
  collectorBearer: string;
};
const fresh = (): Draft => ({
  host: {
    id: "",
    name: "",
    address: "",
    port: 22,
    user: "",
    host_key: "",
    sudo: false,
    enabled: true,
    version: 0,
    target: {
      id: 0,
      name: "mysql",
      kind: "mysql",
      url: "mysql://localhost:3306",
      username: "",
      database: "",
      tls_mode: "require",
      interval_seconds: 30,
      enabled: true,
      labels: { environment: "intranet" },
      last_scrape: 0,
      last_error: "",
      samples: 0,
      duration_ms: 0,
    },
    last_seen: 0,
    agent_version: "",
    agent_state: "",
    agent_os: "",
    agent_arch: "",
    applied_version: 0,
    samples: 0,
  },
  password: "",
  privateKey: "",
  collectorPassword: "",
  collectorBearer: "",
});
const stateText = (host: Host) =>
  host.revoked
    ? text(
        "Access revoked; reinstall before enabling",
        "接入已撤销，需重新部署后启用",
      )
    : !host.last_seen
      ? text("Awaiting first heartbeat", "等待首次心跳")
      : Date.now() - host.last_seen > 90000
        ? text("Heartbeat expired", "心跳已超时")
        : host.agent_state === "paused"
          ? text("Collection paused", "采集已暂停")
          : host.agent_state === "collection_error"
            ? text("Some resources could not be collected", "部分资源采集失败")
            : text("Collecting", "正在采集");

export default function ManagedHosts({
  notify,
}: {
  notify: (value: string, error?: boolean) => void;
}) {
  const { canAdmin } = useSession();
  const [hosts, setHosts] = useState<Host[]>([]),
    [error, setError] = useState(""),
    [loaded, setLoaded] = useState(false),
    [draft, setDraft] = useState<Draft | null>(null),
    [busy, setBusy] = useState("");
  const [events, setEvents] = useState<{
    host: Host;
    items: { at: number; actor: string; action: string; result: string }[];
  } | null>(null);
  const inFlight = useRef(false);
  const [rules, setRules] = useState<HostAlertRule[]>([]);
  const [rulesState, setRulesState] = useState<"loading" | "ready" | "error">("loading");
  const [pendingRevoke, setPendingRevoke] = useState<Host | null>(null);
  const reload = useCallback(async () => {
    if (inFlight.current) return;
    inFlight.current = true;
    try {
      const items = await api<Host[]>("/api/v1/managed-hosts");
      setHosts(items);
      setLoaded(true);
      setError("");
    } catch (err) {
      setError(message(err));
    } finally {
      inFlight.current = false;
    }
  }, []);
  useEffect(() => {
    if (!canAdmin) return;
    void reload();
    const timer = window.setInterval(() => void reload(), 5000);
    return () => clearInterval(timer);
  }, [canAdmin, reload]);
  useEffect(() => {
    if (!canAdmin) return;
    const abort = new AbortController();
    let busy = false;
    const load = async () => {
      if (busy) return;
      busy = true;
      try {
        const items = await api<HostAlertRule[]>("/api/v1/alerts/rules", { signal: abort.signal });
        if (!Array.isArray(items)) throw new Error("invalid rules");
        if (!abort.signal.aborted) { setRules(items); setRulesState("ready"); }
      } catch { if (!abort.signal.aborted) setRulesState("error"); }
      finally { busy = false; }
    };
    void load();
    const timer = window.setInterval(() => void load(), 30000);
    return () => { abort.abort(); clearInterval(timer); };
  }, [canAdmin]);
  if (!canAdmin)
    return (
      <Alert
        severity="warning"
        title={text("Administrator access required", "仅管理员可管理主机")}
      />
    );
  const field = (key: keyof Host, value: unknown) =>
    setDraft((current) =>
      current
        ? { ...current, host: { ...current.host, [key]: value } }
        : current,
    );
  const target = (key: keyof Target, value: unknown) =>
    setDraft((current) =>
      current
        ? {
            ...current,
            host: {
              ...current.host,
              target: { ...current.host.target, [key]: value },
            },
          }
        : current,
    );
  const save = async (event: FormEvent) => {
    event.preventDefault();
    if (!draft || busy) return;
    setBusy("save");
    const secrets: Record<string, string> = {};
    if (draft.password) secrets.ssh_password = draft.password;
    if (draft.privateKey) secrets.ssh_private_key = draft.privateKey;
    if (draft.collectorPassword)
      secrets.collector_password = draft.collectorPassword;
    if (draft.collectorBearer)
      secrets.collector_bearer_token = draft.collectorBearer;
    try {
      const h = draft.host;
      await api(`/api/v1/managed-hosts${h.id ? `/${h.id}` : ""}`, {
        method: h.id ? "PUT" : "POST",
        body: JSON.stringify({ ...h, secure_settings: secrets }),
      });
      setDraft(null);
      await reload();
      notify(text("Host configuration saved", "主机配置已保存"));
    } catch (err) {
      setError(message(err));
    } finally {
      setBusy("");
    }
  };
  const action = async (
    host: Host,
    kind:
      | "pause"
      | "deploy"
      | "fingerprint"
      | "events"
      | "dashboard"
      | "host-dashboard"
      | "revoke",
  ) => {
    if (busy) return;
    setBusy(host.id);
    try {
      if (kind === "revoke") {
        await api(`/api/v1/managed-hosts/${host.id}/revoke`, {
          method: "POST",
          body: JSON.stringify({ version: host.version }),
        });
        setPendingRevoke(null);
        notify(
          text(
            "Old host credential revoked and collection paused",
            "旧采集器凭据已撤销，采集已暂停",
          ),
        );
      } else if (kind === "pause") {
        await api(`/api/v1/managed-hosts/${host.id}`, {
          method: "PUT",
          body: JSON.stringify({ ...host, enabled: !host.enabled }),
        });
        notify(
          text(
            "Saved; waiting for the agent to apply",
            "已保存，等待采集器应用配置",
          ),
        );
      } else if (kind === "dashboard" || kind === "host-dashboard") {
        const dashboard = await api<{ id: string; name: string }>(
          `/api/v1/managed-hosts/${host.id}/dashboard`,
          { method: "POST", body: JSON.stringify({ language: getLocale(), kind: kind === "host-dashboard" ? "host" : "mysql" }) },
        );
        location.assign(
          `/d/${encodeURIComponent(dashboard.id)}/${encodeURIComponent(dashboard.name)}`,
        );
      } else if (kind === "events") {
        const items = await api<
          { at: number; actor: string; action: string; result: string }[]
        >(`/api/v1/managed-hosts/${host.id}/events`);
        setEvents({ host, items });
      } else if (kind === "fingerprint") {
        const pin = await api<{ fingerprint: string }>(
          `/api/v1/managed-hosts/${host.id}/fingerprint`,
          { method: "POST" },
        );
        setDraft({
          host: { ...host },
          password: "",
          privateKey: "",
          collectorPassword: "",
          collectorBearer: "",
        });
        notify(
          text(
            `Observed fingerprint: ${pin.fingerprint}. Compare with a trusted record before saving.`,
            `观测到指纹：${pin.fingerprint}。请与可信记录核对后填写保存。`,
          ),
        );
      } else {
        await api(`/api/v1/managed-hosts/${host.id}/deploy`, {
          method: "POST",
          body: JSON.stringify({ version: host.version }),
        });
        notify(
          text(
            "Service installed; verify the first heartbeat below",
            "服务已安装，请继续核对首次心跳和实际采集状态",
          ),
        );
      }
      await reload();
    } catch (err) {
      setError(message(err));
    } finally {
      setBusy("");
    }
  };
  return (
    <div className="managed-hosts">
      <div className="page-heading">
        <div>
          <h1>{text("Host monitoring", "主机监控")}</h1>
          <p className="subtle">
            {text(
              "Host resources, databases and collection status.",
              "主机资源、数据库与采集状态。",
            )}
          </p>
        </div>
        <div className="actions">
          <Button onClick={() => void reload()}>
            <RefreshCw size={16} />
            {text("Refresh", "刷新")}
          </Button>
          <Button variant="primary" onClick={() => setDraft(fresh())}>
            <Plus size={16} />
            {text("Add host", "添加主机")}
          </Button>
        </div>
      </div>
      {error && (
        <Alert severity="error" title={text("Operation failed", "操作失败")}>
          {error}
        </Alert>
      )}
      <details className="host-enrollment-help"><summary>{text("How hosts connect", "主机接入说明")}</summary><p className="subtle">
        {text(
          "The installer must be able to reach the SSH target. For private networks, run hosts deploy-local from inside that network; subsequent collection settings and pause/resume use the outbound connection.",
          "安装执行器需要能连到目标 SSH。内网主机可在内网执行 hosts deploy-local 完成首次安装；此后的采集配置、暂停与恢复通过出站连接生效。",
        )}
      </p></details>
      {!loaded && !error && (
        <p role="status">{text("Loading hosts…", "正在加载主机…")}</p>
      )}
      {loaded && hosts.length === 0 && (
        <div className="empty-state">
          <Monitor size={30} />
          <h2>{text("No managed hosts", "尚未接入主机")}</h2>
          <p>
            {text(
              "Add an SSH target and a collector, verify the host key, then install.",
              "添加 SSH 目标与采集源，核对主机指纹后部署。",
            )}
          </p>
        </div>
      )}
      <div className="managed-host-grid">
        {hosts.map((host) => {
          const fresh = host.last_seen > Date.now() - 90000,
            healthy = fresh && host.agent_state === "collecting";
          return (
            <section
              className="chart-panel managed-host-card"
              key={host.id}
              aria-label={host.name}
            >
              <div className="panel-heading">
                <Monitor size={26} className="host-icon" /><div className="host-identity"><h2>{host.name}</h2><p>{host.agent_os === "darwin" ? "macOS" : host.agent_os || "—"} · {host.agent_arch || "—"} · {host.address}</p></div>
                <span className={`status ${healthy ? "healthy" : "warning"}`}>
                  <i />
                  {stateText(host)}
                </span>
                <Button size="sm" disabled={!!busy} onClick={() => void action(host,"host-dashboard")}><ArrowUpRight size={15} />{text("Open dashboard", "打开仪表盘")}</Button>
              </div>
              <div className="host-view-nav"><span aria-current="page">{text("Host resources", "主机资源")}</span>
                {host.target.kind === "mysql" ? <button disabled={!!busy} onClick={() => void action(host,"dashboard")}>MySQL <ArrowUpRight size={13} /></button> : null}
                <small>{text("Last heartbeat", "最近心跳")} {host.last_seen ? timeAgo(host.last_seen) : "—"}</small>
              </div>
              <HostResources id={host.id} resources={host.resources} enabled={host.enabled && !host.revoked && fresh && host.collect_host !== false} interval={host.target.interval_seconds} rules={rules} rulesState={rulesState} />
              <details className="host-more"><summary><MoreHorizontal size={17} />{text("Configuration and operations", "配置与操作")}</summary>
              <dl>
                <dt>SSH</dt>
                <dd>
                  {host.user}@{host.address}:{host.port}
                </dd>
                <dt>{text("Source", "采集源")}</dt>
                <dd>
                  {host.target.kind} · {host.target.url}
                </dd>
                <dt>{text("Last heartbeat", "最近心跳")}</dt>
                <dd>{host.last_seen ? timeAgo(host.last_seen) : "—"}</dd>
                <dt>{text("Agent", "采集器")}</dt>
                <dd>
                  {host.agent_version
                    ? `${host.agent_version} · ${host.agent_os}/${host.agent_arch}`
                    : "—"}
                </dd>
                <dt>{text("Configuration", "配置版本")}</dt>
                <dd>
                  {host.applied_version} / {host.version}{" "}
                  {host.applied_version !== host.version
                    ? text("pending", "待应用")
                    : ""}
                </dd>
                <dt>{text("Latest batch", "最近一批")}</dt>
                <dd>{host.samples}</dd>
              </dl>
              <div className="actions">
                <Button
                  onClick={() =>
                    setDraft({
                      host: { ...host, target: { ...host.target } },
                      password: "",
                      privateKey: "",
                      collectorPassword: "",
                      collectorBearer: "",
                    })
                  }
                >
                  <Pencil size={14} />
                  {text("Edit", "编辑")}
                </Button>
                <Button
                  disabled={!!busy}
                  onClick={() => void action(host, "pause")}
                >
                  {host.enabled
                    ? text("Pause", "暂停采集")
                    : text("Resume", "恢复采集")}
                </Button>
                <Button
                  disabled={!!busy || !host.host_key}
                  onClick={() => void action(host, "deploy")}
                >
                  {text("Install / update", "部署 / 更新")}
                </Button>
                <Button
                  disabled={!!busy}
                  onClick={() => void action(host, "events")}
                >
                  {text("Events", "操作记录")}
                </Button>
                <Button
                  variant="destructive"
                  disabled={!!busy || host.revoked}
                  onClick={() => setPendingRevoke(host)}
                >
                  {text("Revoke access", "撤销接入")}
                </Button>
                {host.target.kind === "mysql" && (
                  <Button
                    disabled={!!busy}
                    onClick={() => void action(host, "dashboard")}
                  >
                    {text("Open dashboard", "查看仪表盘")}
                  </Button>
                )}
              </div>
              {!host.host_key && (
                <Button
                  disabled={!!busy}
                  onClick={() => void action(host, "fingerprint")}
                >
                  <ShieldCheck size={14} />
                  {text("Inspect host key", "查看主机指纹")}
                </Button>
              )}
              </details>
            </section>
          );
        })}
      </div>
      {pendingRevoke && (
        <AppModal
          title={text("Revoke host access", "撤销主机接入")}
          onClose={() => {
            if (!busy) setPendingRevoke(null);
          }}
        >
          <p>{pendingRevoke.name}</p>
          <p>
            {text(
              "The old collector credential stops working immediately and collection pauses. Reinstall the collector before enabling it again. Existing measurements and dashboards are retained.",
              "旧采集器凭据会立即失效，并暂停采集。重新接入需要部署后再启用；已有指标和仪表盘保留。",
            )}
          </p>
          <div className="dialog-actions">
            <Button disabled={!!busy} onClick={() => setPendingRevoke(null)}>
              {text("Cancel", "取消")}
            </Button>
            <Button
              variant="destructive"
              disabled={!!busy}
              onClick={() => void action(pendingRevoke, "revoke")}
            >
              {text("Revoke access", "撤销接入")}
            </Button>
          </div>
        </AppModal>
      )}
      {draft && (
        <AppModal
          title={text("Host and collector", "主机与采集源")}
          className="managed-host-form"
          onClose={() => {
            if (!busy) setDraft(null);
          }}
        >
          <form onSubmit={(event) => void save(event)}>
            <div className="form-grid">
              <Field label={text("Name", "名称")}>
                <Input
                  required
                  value={draft.host.name}
                  onChange={(e) => field("name", e.target.value)}
                />
              </Field>
              <Field label={text("SSH address", "SSH 地址")}>
                <Input
                  required
                  value={draft.host.address}
                  placeholder="hostname / IP"
                  onChange={(e) => field("address", e.target.value)}
                />
              </Field>
              <Field label={text("SSH port", "SSH 端口")}>
                <Input
                  type="number"
                  min={1}
                  max={65535}
                  required
                  value={draft.host.port}
                  onChange={(e) => field("port", Number(e.target.value))}
                />
              </Field>
              <Field label={text("SSH user", "SSH 用户")}>
                <Input
                  required
                  autoComplete="off"
                  value={draft.host.user}
                  onChange={(e) => field("user", e.target.value)}
                />
              </Field>
              <Field
                label={text(
                  "SSH password (blank keeps saved)",
                  "SSH 密码（留空保留）",
                )}
              >
                <Input
                  type="password"
                  autoComplete="new-password"
                  value={draft.password}
                  onChange={(e) =>
                    setDraft({ ...draft, password: e.target.value })
                  }
                />
              </Field>
              <Field
                label={text(
                  "Verified SHA256 host fingerprint",
                  "已核对的 SSH SHA256 指纹",
                )}
              >
                <Input
                  autoComplete="off"
                  value={draft.host.host_key}
                  onChange={(e) => field("host_key", e.target.value)}
                />
              </Field>
            </div>
            <label className="checkbox-field">
              <input
                type="checkbox"
                checked={draft.host.sudo}
                onChange={(e) => field("sudo", e.target.checked)}
              />
              {text(
                "Allow sudo to install a system service",
                "允许使用 sudo 安装系统服务",
              )}
            </label>
            <h3>{text("Read-only collector", "只读采集")}</h3>
            <label className="checkbox-field"><input type="checkbox" checked={draft.host.collect_host !== false} onChange={event => field("collect_host",event.target.checked)} />{text("Collect host CPU, memory and writable disks", "同时采集主机 CPU、内存与可写磁盘")}</label>
            <div className="form-grid">
              <Field label={text("Type", "类型")}>
                <select
                  value={draft.host.target.kind || "mysql"}
                  onChange={(e) => {
                    const kind = e.target.value;
                    setDraft({
                      ...draft,
                      host: {
                        ...draft.host,
                        target: {
                          ...draft.host.target,
                          kind,
                          name: kind,
                          url:
                            kind === "mysql"
                              ? "mysql://localhost:3306"
                              : kind === "postgresql"
                                ? "postgresql://localhost:5432"
                                : kind === "redis"
                                  ? "redis://localhost:6379"
                                  : "http://localhost:9100/metrics",
                          tls_mode: kind === "mysql" ? "require" : "",
                        },
                      },
                    });
                  }}
                >
                  <option value="mysql">MySQL / MariaDB</option>
                  <option value="postgresql">PostgreSQL</option>
                  <option value="redis">Redis</option>
                  <option value="prometheus">Prometheus exporter</option>
                </select>
              </Field>
              <Field
                label={text(
                  "Collector address on target host",
                  "目标主机可访问的采集地址",
                )}
              >
                <Input
                  required
                  value={draft.host.target.url}
                  onChange={(e) => target("url", e.target.value)}
                />
              </Field>
              <Field label={text("Database user", "数据库用户")}>
                <Input
                  autoComplete="off"
                  value={draft.host.target.username || ""}
                  onChange={(e) => target("username", e.target.value)}
                />
              </Field>
              <Field
                label={text(
                  "Database password (blank keeps saved)",
                  "数据库密码（留空保留）",
                )}
              >
                <Input
                  type="password"
                  autoComplete="new-password"
                  value={draft.collectorPassword}
                  onChange={(e) =>
                    setDraft({ ...draft, collectorPassword: e.target.value })
                  }
                />
              </Field>
              <Field label={text("Interval in seconds", "采集间隔（秒）")}>
                <Input
                  type="number"
                  min={5}
                  max={86400}
                  required
                  value={draft.host.target.interval_seconds}
                  onChange={(e) =>
                    target("interval_seconds", Number(e.target.value))
                  }
                />
              </Field>
              <Field label={text("Environment", "环境")}>
                <select
                  value={draft.host.target.labels?.environment || "intranet"}
                  onChange={(e) =>
                    target("labels", {
                      ...draft.host.target.labels,
                      environment: e.target.value,
                    })
                  }
                >
                  <option value="intranet">{text("Intranet", "内网")}</option>
                  <option value="release">{text("Release", "正式")}</option>
                  <option value="test">{text("Test", "测试")}</option>
                </select>
              </Field>
            </div>
            <details>
              <summary>{text("Advanced", "高级配置")}</summary>
              <Field
                label={text(
                  "SSH private key (blank keeps saved)",
                  "SSH 私钥（留空保留）",
                )}
              >
                <textarea
                  rows={4}
                  autoComplete="off"
                  value={draft.privateKey}
                  onChange={(e) =>
                    setDraft({ ...draft, privateKey: e.target.value })
                  }
                />
              </Field>
              <Field label={text("Database name", "数据库名")}>
                <Input
                  value={draft.host.target.database || ""}
                  onChange={(e) => target("database", e.target.value)}
                />
              </Field>
              {["mysql", "postgresql"].includes(
                draft.host.target.kind || "",
              ) && (
                <Field label="TLS">
                  <select
                    value={draft.host.target.tls_mode || "disable"}
                    onChange={(e) => target("tls_mode", e.target.value)}
                  >
                    <option value="verify-full">
                      {text(
                        "Verify certificate and hostname",
                        "校验证书与主机名",
                      )}
                    </option>
                    <option value="require">
                      {text(
                        "Encrypt without certificate verification",
                        "加密，不校验证书",
                      )}
                    </option>
                    <option value="disable">
                      {text("Disabled", "不加密")}
                    </option>
                  </select>
                </Field>
              )}
              <Field label="Exporter Bearer Token">
                <Input
                  type="password"
                  autoComplete="new-password"
                  value={draft.collectorBearer}
                  onChange={(e) =>
                    setDraft({ ...draft, collectorBearer: e.target.value })
                  }
                />
              </Field>
            </details>
            <p className="subtle">
              {text(
                "Saved credentials are encrypted and never returned by host list. Blank fields preserve existing secrets.",
                "凭据加密保存，主机列表不返回原文。编辑时留空保留已有凭据。",
              )}
            </p>
            <div className="dialog-actions">
              <Button
                type="button"
                disabled={!!busy}
                onClick={() => setDraft(null)}
              >
                {text("Cancel", "取消")}
              </Button>
              <Button type="submit" variant="primary" disabled={!!busy}>
                {text("Save host", "保存主机")}
              </Button>
            </div>
          </form>
        </AppModal>
      )}
      {events && (
        <AppModal
          title={`${text("Host events", "主机操作记录")} · ${events.host.name}`}
          onClose={() => setEvents(null)}
        >
          <div className="table-scroll">
            <table>
              <thead>
                <tr>
                  <th>{text("Time", "时间")}</th>
                  <th>{text("Actor", "操作人")}</th>
                  <th>{text("Action", "动作")}</th>
                  <th>{text("Result", "结果")}</th>
                </tr>
              </thead>
              <tbody>
                {events.items.map((item, index) => (
                  <tr key={index}>
                    <td>{formatTimestamp(item.at)}</td>
                    <td>{item.actor}</td>
                    <td>{item.action}</td>
                    <td>{item.result}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Button onClick={() => setEvents(null)}>
            {text("Close", "关闭")}
          </Button>
        </AppModal>
      )}
    </div>
  );
}
