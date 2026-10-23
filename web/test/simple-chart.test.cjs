const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const ts = require("typescript");

function load(file) {
  const exported = {};
  const code = ts.transpileModule(fs.readFileSync(file, "utf8"), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX },
  }).outputText;
  vm.runInNewContext(code, {
    exports: exported,
    require: (id) => id.includes("i18n") ? { getLocale: () => "en", t: (text) => text }
      : id.includes("display-format") ? load("src/display-format.ts") : id === "react" ? { lazy: () => null } : {},
    URLSearchParams,
  });
  return exported;
}

const { formatValue } = load("src/api.ts");
const { storedQueryUnit } = load("src/features/Explore.tsx");
const { simpleSeriesIdentity, simpleSeriesColor } = load("src/components/Chart.tsx");

test("series colors remain valid beyond eight series and stable across label/result ordering", () => {
  const identities = Array.from({ length: 32 }, (_, index) => simpleSeriesIdentity({ instance: `fixture-${index}`, service: "api" }));
  const colors = new Map(identities.map((id) => [id, simpleSeriesColor(id)]));
  for (const id of [...identities].reverse()) {
    assert.match(simpleSeriesColor(id), /^#[0-9a-f]{6}$/i);
    assert.equal(simpleSeriesColor(id), colors.get(id));
  }
  assert.equal(simpleSeriesIdentity({ service: "api", instance: "fixture-0" }), identities[0]);
  assert.notEqual(simpleSeriesIdentity({ instance: "fixture-0", service: "api" }, true), identities[0]);
  assert.ok(new Set(colors.values()).size > 1);
});

test("PromQL units never inherit an unrelated selected metric and explicit units win", () => {
  assert.equal(storedQueryUnit("metricspanel_memory_bytes", "last", "sum(rate(requests_total[5m]))"), "");
  assert.equal(storedQueryUnit("metricspanel_memory_bytes", "rate", "vector(0.75)", "percentunit"), "percentunit");
  assert.equal(storedQueryUnit("custom_seconds", "last", "", ""), "");
  assert.equal(storedQueryUnit("custom_seconds", "last", ""), "seconds");
  assert.equal(storedQueryUnit("custom_bytes", "last", ""), "bytes");
  assert.equal(storedQueryUnit("custom_bytes_total", "rate", ""), "binBps");
  assert.equal(storedQueryUnit("custom_total", "rate", ""), "ops");
  assert.equal(storedQueryUnit("custom_unknown", "last", ""), "");
});

test("duration and small numeric formatting retain real precision, ratios and missing values", () => {
  assert.equal(formatValue(0.003, "seconds"), "3 ms");
  assert.equal(formatValue(0.0000042, "seconds"), "4.2 µs");
  assert.equal(formatValue(0.000000007, "seconds"), "7 ns");
  assert.equal(formatValue(1.25, "seconds"), "1.3 s");
  assert.equal(formatValue(0, "seconds"), "0.0 s");
  assert.equal(formatValue(-0.003, "seconds"), "-3 ms");
  assert.equal(formatValue(0.003), "0.003");
  assert.equal(formatValue(0.75, "percentunit"), "75.0%");
  assert.equal(formatValue(75, "percent"), "75.0%");
  assert.equal(formatValue(2048, "binBps"), "2 KB /s");
  assert.equal(formatValue(NaN, "seconds"), "—");
  assert.equal(formatValue(Infinity), "—");
});
