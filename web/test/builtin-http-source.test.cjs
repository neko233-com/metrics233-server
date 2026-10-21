const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const ts = require("typescript");
const helpers = {};
vm.runInNewContext(ts.transpileModule(fs.readFileSync("src/grafana/builtin-http-source.ts", "utf8"), {
  compilerOptions: { module: ts.ModuleKind.CommonJS },
}).outputText, { exports: helpers });

test("HTTP adapters are bounded to existing Prometheus, Loki and Tempo server handlers", () => {
  for (const type of ["prometheus", "loki", "tempo"]) assert.equal(helpers.isBuiltinHTTPSource(type), true);
  for (const type of ["mysql", "grafana", "plugin", "", "Tempo"]) assert.equal(helpers.isBuiltinHTTPSource(type), false);
});

test("HTTP queries interpolate the supported expression and trace ID fields without changing opaque settings", () => {
  const source = { refId: "A", expr: '$service', query: '$service', traceql: '$service', traceId: '$service', legendFormat: '$service', limit: 10, opaque: { untouched: '$service' } };
  const result = helpers.interpolateHTTPQuery(source, (value) => value.replace('$service', 'checkout'));
  assert.equal(result.traceql, 'checkout');
  assert.equal(result.query, 'checkout');
  assert.equal(result.expr, 'checkout');
  assert.equal(result.traceId, 'checkout');
  assert.equal(result.legendFormat, 'checkout');
  assert.equal(result.limit, 10);
  assert.equal(result.opaque, source.opaque);
  assert.equal(source.query, '$service');
  assert.equal(helpers.interpolateHTTPQuery({ refId: 'B' }, (value) => value).expr, undefined);
});
