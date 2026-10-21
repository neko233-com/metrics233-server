const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const ts = require("typescript");
const helpers = {};
vm.runInNewContext(ts.transpileModule(fs.readFileSync("src/render/series-presentation.ts", "utf8"), {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
}).outputText, { exports: helpers });
const present = (inputs, locale = "zh") => helpers.seriesPresentations(inputs, locale);
const plain = value => JSON.parse(JSON.stringify(value));

test("uptime replaces SDK label soup, with exact source and every label retained for details", () => {
  const labels = Object.freeze({ __name__: "metricspanel_uptime_seconds", instance: "random-host:7333", job: "self" });
  const [item] = present([{ labels }]);
  assert.equal(item.name, "监控服务运行时长");
  assert.equal(item.known, true);
  assert.match(item.description, /重启/);
  assert.equal(item.rawName, 'metricspanel_uptime_seconds{instance="random-host:7333",job="self"}');
  assert.deepEqual(plain(item.rawLabels), labels);
  assert.notEqual(item.rawLabels, labels);
  assert.equal(present([{ labels }], "en")[0].name, "Monitoring service uptime");
});

test("explicit field names win; legend templates preserve punctuation and missing labels match datasource rules", () => {
  const input = { metric: "game_players_online", labels: { role: "logic", serverId: "7" },
    displayName: "My exact <alias>", legend: "{{ role }} #{{serverId}} / {{missing}}" };
  assert.equal(present([input])[0].name, "My exact <alias>");
  assert.equal(present([{ ...input, displayName: undefined }])[0].name, "logic #7 / ");
  assert.equal(present([{ ...input, displayName: undefined, legend: "__auto" }])[0].custom, false);
  assert.equal(present([{ ...input, displayName: "", legend: "{{missing}}" }])[0].custom, false);
});

test("business identity retains environment/channel/server/role while hiding instance and job noise", () => {
  const [item] = present([{ metric: "game_players_online", labels: { service: "game", role: "logic", serverId: "0",
    environment: "test", channel: "alpha", instance: "random-node", job: "json" } }]);
  for (const expected of ["当前在线玩家", "角色=逻辑服", "服号=0", "环境=test", "渠道=alpha", "服务=game"]) assert.ok(item.name.includes(expected));
  assert.ok(!item.name.includes("random-node"));
  assert.ok(!item.name.includes("json"));
  assert.match(item.description, /已认证/);
});

test("unknown metrics and expressions retain exact names without a guessed CPU or business interpretation", () => {
  for (const metric of ["custom_cpu_percent", "game_players_online_future", 'rate(node_cpu_seconds_total[5m])', "constructor", "__proto__"]) {
    const [item] = present([{ metric }]);
    assert.equal(item.name, metric);
    assert.equal(item.known, false);
    assert.match(item.description, /暂无/);
  }
  assert.equal(present([{ labels: {}, fallbackName: "Weighted SQL latency" }])[0].name, "Weighted SQL latency");
  assert.equal(present([{ labels: {} }])[0].name, "数值");
});

test("an actual metric name overrides a caller hint and counter definitions never imply a derived percentage", () => {
  const [item] = present([{ metric: "game_players_online", labels: { __name__: "node_cpu_seconds_total", cpu: "0", mode: "idle" } }]);
  assert.equal(item.metric, "node_cpu_seconds_total");
  assert.match(item.name, /^累计 CPU 时间/);
  assert.match(item.description, /需由查询计算/);
  assert.match(helpers.metricPresentation("gateway_connections_active", "zh").description, /不是.*玩家/);
  assert.match(helpers.metricPresentation("game_player_sessions_started_total", "zh").description, /不是去重/);
  assert.match(helpers.metricPresentation("game_sql_calls_total", "zh").description, /不是异步入队/);
  assert.match(helpers.metricPresentation("game_sql_duration_seconds_sum", "zh").description, /除以对应 count/);
});

test("filesystem mounts, SQL operation and geographic dimensions remain meaningful identities", () => {
  const items = present([
    { metric: "node_filesystem_avail_bytes", labels: { mountpoint: "/", device: "disk-a" } },
    { metric: "node_filesystem_avail_bytes", labels: { mountpoint: "/boot/efi", device: "disk-a" } },
    { metric: "game_sql_calls_total", labels: { table: "player", operation: "update" } },
    { metric: "game_players_online_by_region", labels: { country: "Unknown", province: "Unknown", city: "Unknown" } },
  ]);
  assert.ok(items[0].name.includes("挂载点=/"));
  assert.ok(items[1].name.includes("挂载点=/boot/efi"));
  assert.ok(items[2].name.includes("表=player · 操作=update"));
  assert.ok(items[3].name.includes("国家/地区=Unknown"));
  assert.equal(new Set(items.map(item => item.name)).size, 4);
});

test("colliding names restore only the labels needed to distinguish instances and never alter source", () => {
  const inputs = ["node-a", "node-b", "node-c"].map(instance => ({ metric: "metricspanel_uptime_seconds", labels: { instance, job: "self", ignored: "same" } }));
  const before = JSON.stringify(inputs), items = present(inputs);
  assert.equal(new Set(items.map(item => item.name)).size, 3);
  assert.ok(items.every(item => item.name.includes("实例=")));
  assert.ok(items.every(item => !item.name.includes("采集任务=") && !item.name.includes("ignored=")));
  assert.equal(JSON.stringify(inputs), before);
});

test("unknown varying dimensions, missing labels and duplicate aliases are disambiguated", () => {
  const inputs = [{ labels: { shard: "east" } }, { labels: { shard: "west" } }, { labels: {} }].map(input => ({ ...input, metric: "custom_total", displayName: "Custom" }));
  const items = present(inputs);
  assert.equal(new Set(items.map(item => item.name)).size, 3);
  assert.ok(items.every(item => item.name.startsWith("Custom · shard=")));
  assert.ok(items[2].name.includes("∅"));
  assert.ok(items.every(item => item.custom));
});

test("duplicate references and literal suffix aliases still have unique visible names", () => {
  const items = present([
    { displayName: "Alias", reference: "A" }, { displayName: "Alias", reference: "A" },
    { displayName: "Alias #2", reference: "B" }, { displayName: "Alias", reference: "B" },
  ]);
  assert.equal(new Set(items.map(item => item.name)).size, 4);
  assert.ok(items[0].name.includes("A"));
  assert.ok(items[3].name.includes("B"));
});

test("comparison metadata localizes without confusing it with a second live instance", () => {
  const inputs = [{ metric: "game_players_online" }, { metric: "game_players_online", comparison: true }];
  assert.deepEqual(plain(present(inputs).map(item => item.name)), ["当前在线玩家", "当前在线玩家 (比较)"]);
  assert.equal(present([{ displayName: "My alias", comparison: true }], "en")[0].name, "My alias (Comparison)");
  assert.equal(present([{ displayName: "My alias (Comparison)", comparison: true }], "en")[0].name, "My alias (Comparison)");
});

test("long labels are bounded on the legend but remain exact and distinguishable in raw details", () => {
  const common = "x".repeat(400);
  const inputs = ["a", "b"].map(end => ({ metric: "custom_total", labels: { service: common + end, instance: "same" } }));
  const items = present(inputs);
  assert.ok(items.every(item => item.name.length < 150));
  assert.notEqual(items[0].name, items[1].name);
  assert.equal(items[0].rawLabels.service, common + "a");
  assert.ok(items[0].rawName.includes(common + "a"));
});

test("raw selector escapes quotes/newlines and names are plain text, with no prototype-key lookup", () => {
  const labels = JSON.parse('{"__name__":"custom_total","constructor":"safe","__proto__":"plain","label":"quote\\\"\\nline"}');
  const [item] = present([{ labels, displayName: "<img src=x onerror=anything>" }]);
  assert.equal(item.name, "<img src=x onerror=anything>");
  assert.ok(item.rawName.includes('label="quote\\\"\\nline"'));
  assert.equal(item.rawLabels.__proto__, "plain");
  assert.equal(item.rawLabels.constructor, "safe");
});
