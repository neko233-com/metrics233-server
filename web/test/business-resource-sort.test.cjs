const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs"), vm = require("node:vm"), ts = require("typescript");
function compile(file) {
  const exports = {};
  vm.runInNewContext(ts.transpileModule(fs.readFileSync(file, "utf8"), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX } }).outputText,
    { exports, require: name => name === "../i18n" ? { getLocale: () => "zh" } : name === "../ui" ? {} : require(name) });
  return exports;
}
const sort = compile("src/features/BusinessSort.tsx");
test("multi-sort retains priority, toggles numeric direction and never mutates the prior selection", () => {
  const before = [{ column: "avg_ms_window", direction: "desc" }];
  const added = sort.toggleBusinessSort(before, { key: "calls_window", numeric: true }, true);
  assert.deepEqual(JSON.parse(JSON.stringify(added)), [...before, { column: "calls_window", direction: "desc" }]);
  assert.equal(before.length, 1);
  const toggled = sort.toggleBusinessSort(added, { key: "avg_ms_window", numeric: true }, true);
  assert.equal(toggled[0].direction, "asc"); assert.equal(toggled[1].column, "calls_window");
  assert.equal(sort.toggleBusinessSort(added, { key: "table" }, false).length, 1);
});
test("saved sort preferences reject unknown, duplicate and malformed rules", () => {
  const columns = [{ key: "a" }, { key: "b" }];
  assert.equal(sort.validBusinessSort([{ column: "a", direction: "desc" }], columns), true);
  for (const value of [null, [], [{ column: "other", direction: "asc" }], [{ column: "a", direction: "sideways" }], [{ column: "a", direction: "asc" }, { column: "a", direction: "desc" }]]) assert.equal(sort.validBusinessSort(value, columns), false);
});
test("instance panels distinguish process usage and preserve template contents and queries", () => {
  const model = compile("src/features/business-instance-panels.ts");
  const panel = { id: "process-cpu", title: "Process CPU", expr: "unchanged", visualization: "stat", config: { targets: [{ expr: "unchanged", legendFormat: "Process CPU" }] } };
  const localized = model.localizeInstancePanel(panel, "zh");
  assert.equal(localized.title, "进程 CPU · 单核占比"); assert.match(localized.config.description, /多核/);
  assert.equal(localized.config.targets[0].expr, "unchanged"); assert.equal(panel.title, "Process CPU");
  const panels = [panel, { id: "online", title: "Custom player title" }];
  assert.equal(model.instancePanelGroups(panels, true)[0].id, "process");
  assert.equal(model.instancePanelGroups(panels, false).length, 1);
  assert.equal(model.instancePanelGroups(panels, false)[0].panels[1].title, "Custom player title");
});
