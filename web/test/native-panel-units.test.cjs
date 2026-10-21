const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const ts = require("typescript");
const helpers = {};
vm.runInNewContext(ts.transpileModule(fs.readFileSync("src/features/native-panel-units.ts", "utf8"), {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
}).outputText, { exports: helpers });
const plain = value => JSON.parse(JSON.stringify(value));
const panel = display => ({ unit: "bytes", config: { fieldConfig: { defaults: { unit: display, custom: { preserved: true }, thresholds: { steps: [{ color: "dark-red", value: 7 }] } }, overrides: [{ matcher: "keep" }] }, opaque: "keep" } });

test("bytes per second reads back and saves a narrow display override with a valid top-level unit", () => {
  assert.equal(helpers.nativePanelUnitSelection(panel("binBps")), "binBps");
  assert.deepEqual(plain(helpers.nativePanelUnitEdit(panel("bytes"), "binBps")), { unit: "bytes", display_unit: "binBps" });
  assert.deepEqual(plain(helpers.nativePanelUnitEdit(undefined, "binBps")), { unit: "bytes", display_unit: "binBps" });
  assert.deepEqual(plain(helpers.nativePanelUnitEdit(panel("binBps"), "bytes")), { unit: "bytes", display_unit: "" });
});

test("untouched custom units and normal basic-unit mappings remain unchanged", () => {
  const source = panel("custom:widgets"), before = JSON.stringify(source);
  assert.equal(helpers.nativePanelUnitSelection(source), helpers.preserveCustomUnit);
  assert.deepEqual(plain(helpers.nativePanelUnitEdit(source, helpers.preserveCustomUnit)), { unit: "bytes" });
  assert.equal(JSON.stringify(source), before);
  assert.equal(helpers.nativePanelUnitSelection({ unit: "seconds", config: { fieldConfig: { defaults: { unit: "s" } } } }), "seconds");
  assert.equal(helpers.nativePanelUnitSelection({ unit: "count", config: { fieldConfig: { defaults: { unit: "short" } } } }), "count");
  assert.deepEqual(plain(helpers.nativePanelUnitEdit(panel("bytes"), "bytes")), { unit: "bytes" });
  assert.throws(() => helpers.nativePanelUnitEdit(source, "unknown"), /Unsupported panel unit/);
});
