import { useEffect, useRef, useState } from "react";
import { Eye } from "lucide-react";
import { api, jsonBody, message as errorMessage } from "../api";
import { getLocale } from "../i18n";
import "./FeishuNotificationPreview.css";

const text = (en: string, zh: string) => getLocale() === "zh" ? zh : en;
type PlainText = { tag?: string; content?: string };
type CardElement = { tag: string; text?: PlainText; elements?: PlainText[]; actions?: { text?: PlainText; url?: string }[] };
type Preview = { format: string; bytes: number; sent: false; message: {
  msg_type: "interactive" | "text"; content?: { text?: string }; card?: {
    header: { template: string; title: PlainText }; elements: CardElement[];
  };
} };

export function FeishuNotificationPreview({format, title, message, disabled}: {
  format: "interactive" | "text"; title: string; message: string; disabled: boolean;
}) {
  const [status, setStatus] = useState<"firing" | "resolved">("firing");
  const [preview, setPreview] = useState<Preview | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const request = useRef<AbortController | null>(null);
  useEffect(() => {
    request.current?.abort(); setPreview(null); setError(""); setBusy(false);
    return () => request.current?.abort();
  }, [format, title, message, status]);

  return <div className="feishu-preview-editor">
    <div className="feishu-preview-controls">
      <label>{text("Preview state", "预览状态")}
        <select aria-label={text("Preview state", "预览状态")} value={status} disabled={disabled || busy}
          onChange={(event) => setStatus(event.target.value as "firing" | "resolved")}>
          <option value="firing">FIRING · {text("Firing", "触发")}</option>
          <option value="resolved">RESOLVED · {text("Resolved", "恢复")}</option>
        </select>
      </label>
      <button type="button" disabled={disabled || busy} onClick={async () => {
        request.current?.abort(); const controller = new AbortController(); request.current = controller;
        setBusy(true); setError(""); setPreview(null);
        try {
          const value = await api<Preview>("/alerting/contact-points/preview", {method: "POST", signal: controller.signal,
            body: jsonBody({feishu_format: format, status, title_template: title, message_template: message})});
          if (!controller.signal.aborted) setPreview(value);
        } catch (reason) {if (!controller.signal.aborted) setError(errorMessage(reason));}
        finally {if (!controller.signal.aborted) setBusy(false);}
      }}><Eye size={14} />{text(busy ? "Rendering…" : "Preview Feishu message", busy ? "正在渲染…" : "预览飞书消息")}</button>
    </div>
    <p className="notification-help">{text("Sample data only. Preview does not save settings, evaluate rules or send a group message.", "仅使用示例数据。预览不会保存配置、评估规则或向群发送消息。")}</p>
    {error ? <p className="form-error" role="alert">{error}</p> : null}
    {preview ? <section className="feishu-message-preview" aria-label={text("Feishu message preview", "飞书消息预览")}>
      <div className="feishu-preview-caption">{text("PREVIEW · NOT SENT", "预览 · 未发送")} · {(preview.bytes / 1024).toFixed(1)} / 20 KiB</div>
      {preview.message.msg_type === "text" ? <pre>{preview.message.content?.text}</pre> : preview.message.card ? <article>
        <header data-color={preview.message.card.header.template}>{preview.message.card.header.title.content}</header>
        <div className="feishu-preview-body">{preview.message.card.elements.map((element, index) => {
          if (element.tag === "hr") return <hr key={index} />;
          if (element.tag === "div") return <p key={index}>{element.text?.content}</p>;
          if (element.tag === "note") return <small key={index}>{element.elements?.map((value) => value.content).join(" ")}</small>;
          // Preview deliberately renders the trusted action as a visual button,
          // without navigating or accepting URLs from response/user content.
          if (element.tag === "action") return <div key={index} className="feishu-preview-action">{element.actions?.map((value, action) => <span key={action}>{value.text?.content}</span>)}</div>;
          return null;
        })}</div>
      </article> : null}
    </section> : null}
  </div>;
}
