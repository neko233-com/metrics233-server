const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const ts = require("typescript");
const React = require("react");
const { renderToStaticMarkup } = require("react-dom/server");
const helpers = {};
vm.runInNewContext(ts.transpileModule(fs.readFileSync("src/ui/RiskGauge.tsx", "utf8"), {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX },
}).outputText, { exports: helpers, require: name => name.endsWith(".css") ? {} : name === "../i18n" ? { getLocale: () => "zh" } : require(name) });
const render = props => renderToStaticMarkup(React.createElement(helpers.RiskGauge, { label: "CPU 使用率", ...props }));

test("instrument distinguishes a real zero from absent, nonfinite and paused observations", () => {
  const zero = render({ value: 0 });
  assert.match(zero, /role="meter"/); assert.match(zero, /aria-valuenow="0"/); assert.match(zero, /0.0%/);
  for (const value of [null, NaN, Infinity]) {
    const absent = render({ value }); assert.match(absent, /role="img"/); assert.doesNotMatch(absent, /aria-valuenow/); assert.match(absent, /等待有效采样/);
  }
  const stale = render({ value: 99, stale: true }); assert.doesNotMatch(stale, /aria-valuenow/); assert.match(stale, /采集已过期或暂停/);
});
test("instrument supports configured units, reversed risks and percentage thresholds without changing values", () => {
  const bands = helpers.gaugeBands([{ value: null, color: "red" }, { value: 25, color: "green" }], 100, 200, true);
  assert.equal(bands[0].end, .25); assert.equal(bands[1].color, "#39c98a");
  const custom = render({ value: 150, min: 100, max: 200, formatted: "150 MiB", steps: [{ value: null, color: "blue" }] });
  assert.match(custom, /150 MiB/); assert.match(custom, /aria-valuemax="200"/); assert.doesNotMatch(custom, /150.0%/);
});
