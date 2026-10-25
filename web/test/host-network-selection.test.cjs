const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs"), vm = require("node:vm"), ts = require("typescript");
const exportsObject = {};
vm.runInNewContext(ts.transpileModule(fs.readFileSync("src/features/host-network-selection.ts", "utf8"), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText, { exports: exportsObject });
const { recommendHostInterface: recommend, resolveNetworkChoice: resolve } = exportsObject;
const nic = (device, receive_per_second = 0, transmit_per_second = 0) => ({ device, receive_per_second, transmit_per_second });

test("fresh observed traffic wins over idle NICs without summing bridges into physical interfaces", () => {
  const observations = Object.freeze([nic("bridge0", 5000), nic("en1", 12, 10), nic("anpi0"), nic("en0", 5)]);
  assert.equal(recommend(observations, true).device, "en1");
  assert.equal(recommend(observations, false).device, "en0");
  assert.equal(recommend(observations, false).hasTraffic, false);
  assert.equal(recommend([nic("utun1", 100), nic("en0")], true).device, "utun1");
  assert.equal(observations[0].device, "bridge0");
});

test("missing, invalid and zero rates remain distinct from a positive recommendation", () => {
  for (const value of [null, NaN, Infinity, -1, "999"]) {
    assert.equal(recommend([nic("en0", value), nic("en1", null)], true).hasTraffic, false);
  }
  assert.equal(recommend([nic("en0"), nic("en1", Number.MAX_VALUE, Number.MAX_VALUE)], true).device, "en1");
  assert.equal(recommend([], true).device, "");
});

test("automatic baseline can upgrade once, then retains its identity through idle/bursty refreshes", () => {
  const waiting = resolve(null, "host-a", ["en0", "en1"], "en0", false, true);
  assert.equal(waiting.pinned, false);
  const selected = resolve(waiting, "host-a", ["en0", "en1"], "en1", true, true);
  assert.equal(selected.device, "en1"); assert.equal(selected.pinned, true);
  assert.equal(resolve(selected, "host-a", ["en0", "en1"], "en0", true, true), selected);
  assert.equal(resolve(selected, "host-a", [], "", false, false), selected);
});

test("manual idle or absent NIC never silently changes; a different host cannot inherit it", () => {
  const manual = { scope: "host-a", device: "en0", manual: true, pinned: true };
  assert.equal(resolve(manual, "host-a", ["en1"], "en1", true, true), manual);
  assert.equal(resolve(manual, "host-a", [], "", false, false), manual);
  const other = resolve(manual, "host-b", ["eth0"], "eth0", true, true);
  assert.equal(other.device, "eth0"); assert.equal(other.manual, false); assert.equal(other.scope, "host-b");
});

test("automatic missing NIC can recover from a fresh observed alternative", () => {
  const old = { scope: "host-a", device: "en0", manual: false, pinned: true };
  assert.equal(resolve(old, "host-a", ["en1"], "en1", true, false), old);
  const replacement = resolve(old, "host-a", ["en1"], "en1", true, true);
  assert.equal(replacement.device, "en1"); assert.equal(replacement.manual, false);
});
