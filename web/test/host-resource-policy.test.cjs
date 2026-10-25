const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs"), vm = require("node:vm"), ts = require("typescript");
const React = require("react"), { renderToStaticMarkup } = require("react-dom/server");
const compile = (file, overrides) => {
  const exports = {};
  vm.runInNewContext(ts.transpileModule(fs.readFileSync(file, "utf8"), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX } }).outputText,
    { exports, require: name => overrides[name] || (name === "../i18n" ? { getLocale: () => "zh" } : name.endsWith(".css") ? {} : require(name)), AbortController, URLSearchParams });
  return exports;
};
const gauge = compile("src/ui/RiskGauge.tsx", {});
const clock = compile("src/observation-clock.ts", {});
const host = compile("src/features/HostResources.tsx", { "../observation-clock": clock, "./HostNetworkResources": { default: () => null }, "../ui/RiskGauge": gauge, "../api": { api: () => { throw new Error("render must not perform I/O"); }, formatValue: value => String(value), timeAgo: () => "刚刚" } });
const rule = () => ({ uid: "fixture", labels: { managed_host: "one", resource: "cpu", severity: "warning" }, annotations: { threshold: "outdated display annotation", unit: "%" }, paused: false, for_seconds: 600, keep_firing_for_seconds: 300, notification_cooldown_seconds: 3600,
  grafana: { condition: "B", data: [{ refId: "A", model: { expr: 'host_cpu_usage_percent{managed_host="one"}' } }, { refId: "B", model: { type: "threshold", expression: "A", conditions: [{ evaluator: { type: "gt", params: [85] }, unloadEvaluator: { type: "lt", params: [80] } }] } }] } });

test("host guidance reads the actual condition graph and durations, not stale descriptive annotations", () => {
  const policy = host.hostAlertPolicy([rule()], "one", "cpu");
  assert.equal(policy.threshold, 85); assert.equal(policy.recovery, 80); assert.equal(policy.pending, 600); assert.equal(policy.recovering, 300); assert.equal(policy.cooldown, 3600);
});
test("host guidance excludes paused, foreign, acceptance and unsupported conditions", () => {
  const variants = [r => r.paused = true, r => r.labels.managed_host = "other", r => r.labels.scope = "acceptance", r => r.grafana.data = {}, r => r.grafana.data[1].model.conditions = [null], r => r.grafana.data[1].model.conditions[0].evaluator.type = "lt", r => r.grafana.data[1].model.conditions[0].evaluator.params = [NaN], r => r.grafana.data[0].model.expr = "unrelated_metric"];
  for (const change of variants) { const value = rule(); change(value); assert.equal(host.hostAlertPolicy([value], "one", "cpu"), null); }
});
test("host cards explain free capacity and sustained notification rules; paths stay in technical details", () => {
  const resources = { observed_at: Date.now(), cpu_percent: 8, cores: 10, memory_percent: 60, memory_total: 1000, memory_available: 400, uptime: 3600, disks: [{ mountpoint: "/System/Volumes/Data", device: "disk", fstype: "apfs", total: 10000, available: 7000, used_percent: 30 }] };
  const markup = renderToStaticMarkup(React.createElement(host.HostResources, { id: "one", resources, interval: 30, enabled: true, rules: [rule()], rulesState: "ready" }));
  for (const value of ["还可用", "还剩", "CPU 使用率", "系统数据盘", "10分钟", "5分钟", "1小时", "黄色虚线", "采样口径与技术信息"]) assert.ok(markup.includes(value), value);
  assert.doesNotMatch(markup.split("<details")[0].replace(/<[^>]+>/g, ""), /\/System\/Volumes\/Data/);
  const stale = renderToStaticMarkup(React.createElement(host.HostResources, { id: "one", resources, interval: 30, enabled: false, rules: [], rulesState: "error" }));
  assert.doesNotMatch(stale, /role="meter"/); assert.match(stale, /上次采样/); assert.match(stale, /暂时无法读取规则/);
});
