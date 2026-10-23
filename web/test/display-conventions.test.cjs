const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs"), vm = require("node:vm"), ts = require("typescript");
function load(file) {
  const exports = {};
  vm.runInNewContext(ts.transpileModule(fs.readFileSync(file, "utf8"), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText, { exports, Date, Intl });
  return exports;
}
const display = load("src/display-format.ts");
const directory = load("src/features/business-instance-directory-model.ts");

test("capacity uses decimal units, retains zero and sign, rejects invalid values", () => {
  assert.equal(display.formatBytes(1000000), "1 MB");
  assert.equal(display.formatBytes(1073741824), "1.1 GB");
  assert.equal(display.formatBytes(-1500000), "-1.5 MB");
  assert.equal(display.formatBytes(0), "0 B");
  assert.equal(display.formatBytes(NaN), "—");
});

test("full timestamps follow local calendar fields and keep midnight zero-padded", () => {
  const local = new Date(2026, 9, 23, 0, 2, 3);
  assert.equal(display.formatTimestamp(local), "2026-10-23 00:02:03");
  assert.equal(display.formatTimestamp(local.getTime()), display.formatTimestamp(local));
  assert.equal(display.formatTimestamp(local.toISOString()), display.formatTimestamp(local));
  assert.equal(display.formatChartTimestamp(local.getTime()), "2026-10-23\n00:02:03");
  assert.equal(display.formatTimestamp("invalid"), "—");
  assert.equal(display.formatTimestamp(undefined), "—");
  assert.notEqual(display.formatTimestamp(0), "—");
});

test("GMT display supports positive, negative and half/quarter-hour computer offsets", () => {
  assert.equal(display.browserTimezoneLabel({ getTimezoneOffset: () => -480 }), "GMT+8");
  assert.equal(display.browserTimezoneLabel({ getTimezoneOffset: () => 240 }), "GMT-4");
  assert.equal(display.browserTimezoneLabel({ getTimezoneOffset: () => -345 }), "GMT+5:45");
  assert.equal(display.browserTimezoneLabel({ getTimezoneOffset: () => 0 }), "GMT+0");
});

test("directory ordering is natural, stable under polling and never mutates API records", () => {
  const make = (id, server, last) => ({ id, last_seen: last, reported_ip: "", observed_ip: "", identity: { serverId: server, service: "game", instance: id } });
  const input = [make("b", "10", 2), make("c", "2", 3), make("a", "2", 1)];
  assert.deepEqual(Array.from(directory.sortDirectory(input, "server"), item => item.id), ["a", "c", "b"]);
  input[0].last_seen = 4;
  assert.deepEqual(Array.from(directory.sortDirectory(input, "server"), item => item.id), ["a", "c", "b"]);
  assert.deepEqual(Array.from(directory.sortDirectory(input, "recent"), item => item.id), ["b", "c", "a"]);
  assert.equal(input[0].id, "b");
  assert.equal(directory.reportFresh(0, 100000), false);
  assert.equal(directory.reportFresh(10000, 100000), true);
  assert.equal(directory.reportFresh(9999, 100000), false);
  assert.equal(directory.reportFresh(150000, 100000), false);
});
