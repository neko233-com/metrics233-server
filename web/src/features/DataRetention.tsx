import { useEffect, useState, type FormEvent } from "react";
import { api, ApiError, jsonBody, message } from "../api";
import { getLocale } from "../i18n";
import { Alert, Button, Field, Input } from "../ui";
import { useSession } from "../ui/session";

type Retention = { days: number; version: number; editable: boolean; backend: string; cleanup_interval_seconds: number };
const text = (en: string, zh: string) => getLocale() === "zh" ? zh : en;

export function DataRetention({ notify, reload }: { notify: (s: string, error?: boolean) => void; reload: () => Promise<void> }) {
  const { canAdmin } = useSession();
  const [saved, setSaved] = useState<Retention | null>(null);
  const [days, setDays] = useState("15");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setError("");
    void api<Retention>("/settings/retention", { signal: controller.signal })
      .then(value => { setSaved(value); setDays(String(value.days)); })
      .catch(err => { if (!controller.signal.aborted) setError(message(err)); });
    return () => controller.abort();
  }, [revision]);
  async function save(event: FormEvent) {
    event.preventDefault();
    if (!saved || !canAdmin || !saved.editable || busy) return;
    const count = Number(days);
    if (!Number.isInteger(count) || count < 1 || count > 3650) {
      setError(text("Enter a whole number from 1 to 3650.", "请输入 1–3650 之间的整数。")); return;
    }
    setBusy(true); setError("");
    try {
      const value = await api<Retention>("/settings/retention", { method: "PUT", body: jsonBody({ days: count, version: saved.version }) });
      setSaved(value); setDays(String(value.days));
      notify(text("Data retention saved", "数据保留设置已保存"));
      await reload();
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setSaved(null); setRevision(value => value + 1);
        notify(text("Retention changed elsewhere. Reloaded the latest setting; review before saving again.", "保留设置已被其他管理员修改，已重新加载；请核对后再次保存。"), true);
      } else setError(message(err));
    } finally { setBusy(false); }
  }
  return <section className="list-panel" aria-labelledby="data-retention-title">
    <div className="section-heading"><h2 id="data-retention-title">{text("Storage & retention", "存储与保留")}</h2></div>
    <form onSubmit={save} style={{ padding: "0 20px 20px", maxWidth: 620 }}>
      <p className="subtle">{text("Keep recent metric samples. Expired samples are permanently deleted at startup and hourly; accounts, dashboards and alert configuration are retained.", "只保留最近的指标数据。启动时及每小时自动永久删除过期样本；账号、仪表盘与告警配置会保留。")}</p>
      {saved?.backend === "embedded" && <p>{text("Compressed time series · XOR blocks + Zstd WAL. Built-in vector database · HNSW search + Zstd storage. Configuration is stored separately.", "压缩时序 · XOR 数据块 + Zstd 日志。内置向量库 · HNSW 检索 + Zstd 存储。配置独立保存。")}</p>}
      {saved?.backend === "sqlite" && <p className="subtle">{text("Legacy SQLite metric storage. Stop the service and run storage migrate to enable compressed storage.", "当前使用兼容版 SQLite 指标存储；停服执行 storage migrate 后可启用压缩存储。")}</p>}
      {error && <Alert severity="error" title={error} />}
      <Field label={text("Retention days", "保留天数")}>
        <Input type="number" min={1} max={3650} step={1} required value={days}
          disabled={!saved || !canAdmin || !saved.editable || busy} onChange={event => setDays(event.target.value)} />
      </Field>
      {saved && !saved.editable && <p className="subtle">{text("ClickHouse uses its explicitly configured table TTL. Change its TTL and startup retention together.", "ClickHouse 使用表上配置的 TTL；请同时修改数据库 TTL 与启动保留期。")}</p>}
      {!canAdmin && <p className="subtle">{text("Only administrators can change retention.", "仅管理员可修改保留天数。")}</p>}
      <Button type="submit" disabled={!saved || !canAdmin || !saved.editable || busy || Number(days) === saved.days}>
        {busy ? text("Saving…", "保存中…") : text("Save retention", "保存保留设置")}
      </Button>
    </form>
  </section>;
}
