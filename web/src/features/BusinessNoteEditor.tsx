import { useEffect, useRef, useState } from "react";
import { api, ApiError, jsonBody, message } from "../api";
import { getLocale } from "../i18n";
import { Alert, AppModal, Button, Field } from "../ui";
import type { BusinessInstance, BusinessNote } from "./business-instance-directory-model";

const text = (en: string, zh: string) => getLocale() === "zh" ? zh : en;
export default function BusinessNoteEditor({ item, close, saved }: { item: BusinessInstance; close: () => void; saved: (note: BusinessNote) => void }) {
  const [description, setDescription] = useState(item.note?.description || "");
  const [version, setVersion] = useState(item.note?.version || 0);
  const [error, setError] = useState("");
  const [conflict, setConflict] = useState<BusinessNote | null>(null);
  const [busy, setBusy] = useState(false);
  const request = useRef<AbortController | null>(null);
  useEffect(() => () => request.current?.abort(), []);
  const bytes = new TextEncoder().encode(description).length;
  async function save() {
    if (busy || conflict || bytes > 4096) return;
    const controller = new AbortController(); request.current = controller; setBusy(true); setError("");
    try {
      const note = await api<BusinessNote>(`/business/instances/${item.id}/note`, { method: "PUT", signal: controller.signal, body: jsonBody({ version, description }) });
      if (!controller.signal.aborted) { saved(note); close(); }
    } catch (cause) {
      if (!controller.signal.aborted) {
        if (cause instanceof ApiError && cause.status === 409) {
          setError(text("This note was edited elsewhere. Compare the current text before saving.", "备注已在别处修改。请先核对最新内容，草稿已保留。"));
          try { const latest = await api<BusinessInstance>(`/business/instances/${item.id}`, { signal: controller.signal }); if (!controller.signal.aborted) setConflict(latest.note || { version: 0, description: "", updated_at: 0 }); } catch (readError) { if (!controller.signal.aborted) setError(message(readError)); }
        } else setError(message(cause));
      }
    } finally { if (!controller.signal.aborted) setBusy(false); }
  }
  return <AppModal title={text("Edit instance note", "编辑实例备注")} className="business-note-dialog" onClose={close}>
    <form onSubmit={event => { event.preventDefault(); void save(); }}>
      <p className="subtle">{item.identity.service} / {item.identity.serverId}</p>
      <Field label={text("Description", "备注")}><textarea aria-label={text("Instance description", "实例备注")} rows={5} value={description} onChange={event => setDescription(event.target.value)} placeholder={text("Purpose, owner, maintenance notes…", "用途、负责人、维护说明，内容自由填写…")} /></Field>
      <span className={bytes > 4096 ? "error" : "subtle"}>{bytes} / 4096 {text("bytes", "字节")}</span>
      {error && <Alert severity="error" title={text("Note not saved", "备注尚未保存")}>{error}</Alert>}
      {conflict && <div className="business-note-conflict"><strong>{text("Current saved note", "当前已保存的备注")}</strong><p>{conflict.description || text("Empty", "空备注")}</p><Button type="button" onClick={() => { setDescription(conflict.description); setVersion(conflict.version); setConflict(null); setError(""); }}>{text("Use current text", "采用最新内容")}</Button><Button type="button" onClick={() => { setVersion(conflict.version); setConflict(null); setError(""); }}>{text("Keep my draft and review again", "保留我的草稿，重新核对保存")}</Button></div>}
      <div className="business-note-actions"><Button type="button" onClick={close}>{text("Cancel", "取消")}</Button><Button type="submit" variant="primary" disabled={busy || !!conflict || bytes > 4096}>{busy ? text("Saving…", "正在保存…") : text("Save note", "保存备注")}</Button></div>
    </form>
  </AppModal>;
}
