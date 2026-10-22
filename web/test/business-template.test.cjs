const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const ts = require("typescript");
const helpers = {};
vm.runInNewContext(ts.transpileModule(fs.readFileSync("src/features/business-template-model.ts", "utf8"), {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
}).outputText, { exports: helpers });
const instance = (id, mode = "inherit", template = "template") => ({ id, identity: { environment: id }, binding: { mode, dashboard_id: template } });
const dashboard = current => ({ variables: [{ name: "business_scope", current }] });

test("only an unbound native scope enters template preview; concrete independent copies keep their normal view", () => {
  assert.equal(helpers.isBusinessTemplateCandidate(dashboard('service="__unbound_template__"')), true);
  assert.equal(helpers.isBusinessTemplateCandidate(dashboard("")), true);
  assert.equal(helpers.isBusinessTemplateCandidate(dashboard('service="actual",environment="release"')), false);
  assert.equal(helpers.isBusinessTemplateCandidate({ ...dashboard(""), grafana: { templating: {} } }), false);
  assert.equal(helpers.isBusinessTemplateCandidate({ name: "网关业务模板（Metrics233托管）" }), false);
});

test("preview accepts exact live inheritance, never another template or independent/default bindings", () => {
  const values = [instance("release"), instance("test"), instance("copy", "independent"), instance("builtin", "default"), instance("other", "inherit", "other")];
  assert.deepEqual(Array.from(helpers.boundTemplateInstances(values, "template"), item => item.id), ["release", "test"]);
  assert.equal(helpers.isResolvedTemplateInstance({ instance: values[0], dashboard: { id: "template" } }, "release", "template"), true);
  assert.equal(helpers.isResolvedTemplateInstance({ instance: values[1], dashboard: { id: "template" } }, "release", "template"), false);
  assert.equal(helpers.isResolvedTemplateInstance({ instance: values[2], dashboard: { id: "template" } }, "copy", "template"), false);
});

test("auto-preview requires one complete first page; explicit selections survive refresh but never cross instances", () => {
  assert.equal(helpers.templateSelection([instance("only")], "", "", ""), "only");
  assert.equal(helpers.templateSelection([instance("only")], "next", "", ""), "");
  assert.equal(helpers.templateSelection([instance("only")], "", "before", ""), "");
  assert.equal(helpers.templateSelection([instance("release"), instance("test")], "", "", ""), "");
  assert.equal(helpers.templateSelection([instance("release"), instance("test")], "", "", "test"), "test");
  assert.equal(helpers.templateSelection([instance("release"), instance("test")], "", "", "deleted"), "");
});

test("environment groups keep release, test, shared and unknown identities separate without mutating input", () => {
  const grouped = {};
  vm.runInNewContext(ts.transpileModule(fs.readFileSync("src/features/business-instance-groups.ts", "utf8"), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText, { exports: grouped });
  const values = [instance("other"), instance("shared"), instance(""), instance("test"), instance("release"), instance("production"), instance("intranet")];
  const result = grouped.groupBusinessInstances(values);
  assert.deepEqual(Array.from(result, group => group.environment), ["release", "test", "intranet", "shared", "", "other", "production"]);
  assert.deepEqual(Array.from(values, item => item.id), ["other", "shared", "", "test", "release", "production", "intranet"]);
});
