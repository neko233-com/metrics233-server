const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const ts = require("typescript");
const helpers = {};
vm.runInNewContext(ts.transpileModule(fs.readFileSync("src/render/traces/trace-detail.ts", "utf8"), {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
}).outputText, { exports: helpers });
const plain = (value) => JSON.parse(JSON.stringify(value));
const span = (id, parent = "", start = 0) => ({ id, parent, start, duration: 1 });

test("trace hierarchy is ordered by ancestry and collapse hides the entire subtree", () => {
  const spans = [span("root"), span("other", "root", 5), span("db", "root", 10), span("wait", "db", 15)];
  assert.deepEqual(plain(helpers.spanTree(spans, new Set()).map((row) => [row.span.id, row.depth])), [["root", 0], ["other", 1], ["db", 1], ["wait", 2]]);
  assert.deepEqual(plain(helpers.spanTree(spans, new Set(["db"])).map((row) => row.span.id)), ["root", "other", "db"]);
  assert.deepEqual(plain(helpers.spanTree(spans, new Set(["root"])).map((row) => row.span.id)), ["root"]);
});

test("partial traces, cycles and a 10000-deep span chain terminate without losing spans", () => {
  const malformed = [span("orphan", "missing"), span("self", "self"), span("cycle-a", "cycle-b"), span("cycle-b", "cycle-a")];
  const rows = helpers.spanTree(malformed, new Set());
  assert.equal(rows.length, 4);
  assert.equal(new Set(rows.map((row) => row.span.id)).size, 4);
  const deep = Array.from({ length: 10000 }, (_, index) => span(String(index), index ? String(index - 1) : "", index));
  const chain = helpers.spanTree(deep, new Set());
  assert.equal(chain.length, 10000);
  assert.equal(chain[9999].depth, 9999);
});

test("span normalization ignores search frames and invalid or duplicated spans", () => {
  const field = (name, values) => ({ name, values });
  const fields = [field("spanID", ["child", "root", "invalid", "root"]), field("traceID", ["trace", "trace"]),
    field("parentSpanID", ["root", ""]), field("startTime", [20, 10, "bad", 10]), field("durationMs", [0.125, 50, 2, 50]),
    field("operationName", ["SELECT", "GET /orders"]), field("status", ["error", "ok"])];
  const spans = helpers.traceSpans([{ fields: [field("traceID", ["search"])] }, { fields }]);
  assert.deepEqual(plain(spans.map((row) => [row.id, row.duration, row.status])), [["root", 50, "ok"], ["child", 0.125, "error"]]);
  assert.equal(helpers.traceDuration(0.125), "125.0 µs");
  assert.equal(helpers.traceDuration(1250), "1.25 s");
});
