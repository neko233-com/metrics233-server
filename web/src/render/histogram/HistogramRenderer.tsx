import { useMemo } from "react";
import { Pencil, Trash2 } from "lucide-react";
import { t } from "../../i18n";
import type { RendererProps } from "../types";
import "./HistogramRenderer.css";
import { histogramBins } from "../discrete/model";

export default function HistogramRenderer({
  panel,
  frames,
  timeInfo,
  loading,
  streaming,
  queryError,
  onEdit,
  onRemove,
}: RendererProps) {
  const config = panel.config;
  const model = useMemo(() => histogramBins(frames), [frames]);
  const bars = model.bins.slice(0, 200);
  const max = useMemo(
    () => bars.reduce((highest, bar) => Math.max(highest, bar.value), 0),
    [bars],
  );
  return (
    <section
      className="chart-panel native-histogram-panel"
      data-bar-count={bars.length}
      aria-label={t(panel.title)}
    >
      <div className="panel-heading">
        <h2 title={config?.description}>{t(panel.title)}</h2>
        <div className="panel-actions">
          {streaming ? (
            <span className="status healthy" role="status">
              <i />
              {t("Live")}
            </span>
          ) : null}
          {onEdit ? (
            <button
              className="icon-button"
              aria-label={`${t("Edit")} ${t(panel.title)}`}
              onClick={onEdit}
            >
              <Pencil size={15} />
            </button>
          ) : null}
          {onRemove ? (
            <button
              className="icon-button danger-hover"
              aria-label={`${t("Remove")} ${t(panel.title)}`}
              onClick={onRemove}
            >
              <Trash2 size={15} />
            </button>
          ) : null}
        </div>
      </div>
      {timeInfo ? <p className="panel-time-info">{timeInfo}</p> : null}
      {model.invalid && <p className="form-error" role="alert">{t("Histogram counts must be non-negative. Cumulative buckets need unique increasing bounds; do not mix them with raw observations.")}</p>}
      {(model.truncated || model.bins.length > bars.length) && <p className="subtle" role="status">{t("Panel display budget reached. Narrow the query to see remaining categories.")}</p>}
      {queryError ? (
        <p className="form-error" role="alert">
          {t(queryError)}
        </p>
      ) : null}
      <div className="native-histogram-viewport">
        {bars.length ? (
          <div className="native-histogram-bars" role="img" aria-label={t(panel.title)}>
            {bars.map((bar, index) => (
              <div className="native-histogram-bar" key={`${bar.label}:${index}`}>
                <span className="native-histogram-value">{bar.value}</span>
                <span
                  className="native-histogram-fill"
                  style={{
                    height: `${max > 0 && bar.value > 0 ? Math.max(2, (bar.value / max) * 100) : 0}%`,
                  }}
                />
                <span className="native-histogram-label" title={bar.label}>
                  {bar.label}
                </span>
              </div>
            ))}
          </div>
        ) : (
          <p className="native-histogram-empty" role="status">
            {loading ? t("Loading metrics…") : t("No samples in this time range")}
          </p>
        )}
      </div>
    </section>
  );
}
