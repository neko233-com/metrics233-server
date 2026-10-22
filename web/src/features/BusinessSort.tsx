import { useEffect, useState } from "react";
import { getLocale } from "../i18n";
import { Button } from "../ui";

export type SortRule = { column: string; direction: "asc" | "desc" };
export type SortColumn = { key: string; label: string; numeric?: boolean };
const text = (en: string, zh: string) => getLocale() === "zh" ? zh : en;

export function toggleBusinessSort(rules: SortRule[], column: SortColumn, append: boolean): SortRule[] {
  const before = rules.find(rule => rule.column === column.key);
  const next: SortRule = { column: column.key, direction: before ? before.direction === "asc" ? "desc" : "asc" : column.numeric ? "desc" : "asc" };
  if (!append) return [next];
  if (before) return rules.map(rule => rule.column === column.key ? next : rule);
  return [...rules, next].slice(0, 5);
}

export function validBusinessSort(value: unknown, columns: SortColumn[]): value is SortRule[] {
  return Array.isArray(value) && value.length > 0 && value.length <= Math.min(columns.length, 5) && value.every(rule => rule && columns.some(column => column.key === rule.column) && (rule.direction === "asc" || rule.direction === "desc")) && new Set(value.map(rule => rule.column)).size === value.length;
}

export function useBusinessSort(kind: string, columns: SortColumn[], defaults: SortRule[]) {
  const storageKey = `metrics233-business-sort-${kind}`;
  const [rules, setRules] = useState<SortRule[]>(() => { try { const value: unknown = JSON.parse(localStorage.getItem(storageKey) || "null"); return validBusinessSort(value, columns) ? value : defaults; } catch { return defaults; } });
  useEffect(() => { try { localStorage.setItem(storageKey, JSON.stringify(rules)); } catch { /* preferences are optional */ } }, [rules, storageKey]);
  return [rules, setRules] as const;
}

export function BusinessSortControls({ rules, columns, onChange, defaults }: { rules: SortRule[]; columns: SortColumn[]; onChange: (rules: SortRule[]) => void; defaults: SortRule[] }) {
  return <details className="business-sort-controls"><summary>{text("Sort rules", "排序规则")} · {rules.map((rule, i) => `${i + 1}. ${columns.find(column => column.key === rule.column)?.label} ${rule.direction === "desc" ? "↓" : "↑"}`).join(" / ")}</summary>
    <p className="subtle">{text("Click a column header for primary sorting. Shift-click to add another rule. Earlier rules have higher priority; missing values always come last.", "点击表头设置主排序，Shift + 点击追加次排序。越靠上优先级越高，缺失值始终放最后。")}</p>
    {rules.map((rule, index) => <div className="business-sort-rule" key={rule.column}><span>{index + 1}</span><select aria-label={text(`Sort column ${index + 1}`, `排序字段 ${index + 1}`)} value={rule.column} onChange={event => onChange(rules.map((value, i) => i === index ? { ...value, column: event.target.value } : value))}>{columns.filter(column => column.key === rule.column || !rules.some(value => value.column === column.key)).map(column => <option key={column.key} value={column.key}>{column.label}</option>)}</select><select aria-label={text(`Sort direction ${index + 1}`, `排序方向 ${index + 1}`)} value={rule.direction} onChange={event => onChange(rules.map((value, i) => i === index ? { ...value, direction: event.target.value as "asc" | "desc" } : value))}><option value="desc">{text("Descending", "从高到低")}</option><option value="asc">{text("Ascending", "从低到高")}</option></select><Button disabled={index === 0} aria-label={text(`Raise priority ${index + 1}`, `提高第 ${index + 1} 条优先级`)} onClick={() => { const next = [...rules]; [next[index - 1], next[index]] = [next[index], next[index - 1]]; onChange(next); }}>{text("Move up", "上移")}</Button><Button disabled={rules.length === 1} onClick={() => onChange(rules.filter((_, i) => i !== index))}>{text("Remove", "移除")}</Button></div>)}
    <div className="business-sort-actions"><Button disabled={rules.length >= Math.min(columns.length, 5)} onClick={() => { const next = columns.find(column => !rules.some(rule => rule.column === column.key)); if (next) onChange([...rules, { column: next.key, direction: next.numeric ? "desc" : "asc" }]); }}>{text("Add sort rule", "添加排序规则")}</Button><Button onClick={() => onChange(defaults)}>{text("Reset sorting", "恢复默认排序")}</Button></div>
  </details>;
}
