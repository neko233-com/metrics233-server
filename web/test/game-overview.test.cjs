const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const ts = require("typescript");
const model = {};
vm.runInNewContext(ts.transpileModule(fs.readFileSync("src/features/game-overview-model.ts", "utf8"), {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
}).outputText, { exports: model });

test("daily gauges are scoped and bounded to a fresh Beijing-day report snapshot", () => {
  const scope = { channel: 'a"b', environment: "test" };
  const daily = model.signalExpression("todayLogins", scope, true);
  assert.ok(daily.includes('target_channel=' + JSON.stringify(scope.channel)));
  assert.match(daily, /target_environment="test"/);
  assert.match(daily, /sdk_today_population_period_start_seconds/);
  assert.match(daily, /scalar\(floor\(vector\(\(time\(\) \+ 28800\) \/ 86400\)\)\)/);
  assert.match(daily, /sdk_today_population_observed_at_seconds.* <= 90/);
  assert.doesNotMatch(model.signalExpression("connections", scope, true), /channel=|environment=/);
  assert.match(model.scopesExpression, /game_players_online.*sdk_today_population_observed_at_seconds/);
  assert.match(model.scopesExpression, /"environment", "\$1", "target_environment"/);
});

test("snapshot preserves observed zero and ignores invalid percentile results", () => {
  const got = model.readSnapshot([
    { metric: { mp_signal: "todayRegistrations" }, value: [1, "0"] },
    { metric: { mp_signal: "todayRegistrations_age" }, value: [1, "5"] },
    { metric: { mp_signal: "loginLatency" }, value: [1, "NaN"] },
  ]);
  assert.equal(got.todayRegistrations.value, 0);
  assert.equal(got.todayRegistrations.age, 5);
  assert.equal(got.loginLatency, undefined);
});

test("Grafana dashboard uses a selectable Prometheus datasource and includes all daily panels", () => {
  const dashboard = model.makeGrafanaGameDashboard("en");
  assert.equal(dashboard.__inputs[0].pluginId, "prometheus");
  assert.ok(dashboard.panels.every(panel => panel.gridPos.x + panel.gridPos.w <= 24));
  assert.equal(new Set(dashboard.panels.map(panel => panel.id)).size, dashboard.panels.length);
  const registrationQuery = dashboard.panels.find(panel => panel.title === "New registrations today").targets[0].expr;
  assert.ok(registrationQuery.includes('target_channel=~"${channel:regex}"'));
  assert.ok(registrationQuery.includes('target_environment=~"${environment:regex}"'));
  assert.match(model.signalExpression("center", { channel: "", environment: "" }), /otel_temporality="cumulative"/);
});
