const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const ts = require("typescript");
const helpers = {};
vm.runInNewContext(
  ts.transpileModule(fs.readFileSync("src/render/discrete/model.ts", "utf8"), {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
    },
  }).outputText,
  { exports: helpers },
);
const plain = (value) => JSON.parse(JSON.stringify(value));
const number = (values) => ({
  name: "value",
  type: "number",
  values,
  config: {},
});
const frame = (fields) => ({ fields });
const describe = (_, value) => ({
  text: String(value),
  color: value === 1 ? "green" : "red",
});

test("numeric reductions preserve real zero, null and signed values without implicit coercion", () => {
  const values = [null, "9", false, NaN, Infinity, 0, -2, 4, null];
  assert.equal(helpers.reduceNumeric(values, "lastNotNull"), 4);
  assert.equal(helpers.reduceNumeric(values, "last"), null);
  assert.equal(helpers.reduceNumeric(values, "first"), null);
  assert.equal(helpers.reduceNumeric(values, "firstNotNull"), 0);
  assert.equal(helpers.reduceNumeric(values, "min"), -2);
  assert.equal(helpers.reduceNumeric(values, "max"), 4);
  assert.equal(helpers.reduceNumeric(values, "sum"), 2);
  assert.equal(helpers.reduceNumeric(values, "mean"), 2 / 3);
  assert.equal(helpers.reduceNumeric(values, "count"), 3);
  assert.equal(helpers.reduceNumeric([null], "count"), 0);
  assert.equal(helpers.reduceNumeric([null, "0", false], "lastNotNull"), null);
  assert.equal(helpers.reduceNumeric([1e308, 1e308], "sum"), null);
});

test("category tables retain every category and numeric field with stable identities and explicit budgets", () => {
  const source = [
    frame([
      { name: "region", type: "string", values: ["a", "b", "c"] },
      number([0, null, -3]),
      { ...number([7, 8, 9]), name: "second" },
    ]),
  ];
  const before = JSON.stringify(source);
  const result = helpers.categories(source);
  assert.equal(result.rows.length, 6);
  assert.deepEqual(plain(result.rows.slice(0, 3).map((row) => row.value)), [
    0,
    null,
    -3,
  ]);
  assert.equal(new Set(result.rows.map((row) => row.id)).size, 6);
  assert.equal(JSON.stringify(source), before);
  const large = helpers.categories([
    frame([
      { name: "category", type: "string", values: Array(5100).fill("x") },
      number(Array(5100).fill(1)),
    ]),
  ]);
  assert.equal(large.rows.length, 5000);
  assert.equal(large.omitted, 100);
  const points = helpers.categories([frame([number(Array(200005).fill(1))])]);
  assert.equal(points.scanned, 200000);
  assert.ok(points.omitted > 0);
});

test("pie proportions do not overflow, never silently accept negatives, and retain shares for all legend rows", () => {
  const rows = helpers.categories([
    frame([
      {
        name: "name",
        type: "string",
        values: Array.from({ length: 240 }, (_, index) => "category " + index),
      },
      number(Array(240).fill(1e308)),
    ]),
  ]).rows;
  const result = helpers.pieParts(rows);
  assert.equal(result.negative, false);
  assert.equal(result.parts.length, 200);
  assert.equal(result.parts.at(-1).members, 41);
  assert.equal(result.parts.at(-1).value, null); // representational overflow is not reported as a real zero
  assert.equal(result.shares.size, 240);
  assert.ok(
    Math.abs(result.parts.reduce((sum, part) => sum + part.share, 0) - 1) <
      1e-12,
  );
  assert.ok(Math.abs(result.shares.get(rows[239].id) - 1 / 240) < 1e-15);
  assert.equal(helpers.pieParts([{ ...rows[0], value: -1 }]).negative, true);
  assert.equal(
    helpers.pieParts([
      { ...rows[0], value: 0 },
      { ...rows[1], value: null },
    ]).parts.length,
    0,
  );
});

test("state observations sort, replace duplicates and preserve missing intervals", () => {
  const fields = [
    {
      name: "Time",
      type: "time",
      values: [1000, 0, 2000, 3000, 3000, 9000, 10000],
    },
    number([1, 1, null, 0, 1, 0, 0]),
  ];
  const result = helpers.stateRows([frame(fields)], 0, 11000, describe);
  assert.equal(result.rows.length, 1);
  const points = result.rows[0].points;
  assert.deepEqual(
    plain(points.map((point) => [point.from, point.to, point.value])),
    [
      [0, 1000, 1],
      [1000, 2000, 1],
      [3000, 5500, 1],
      [9000, 10000, 0],
      [10000, 11000, 0],
    ],
  );
  const merged = helpers.timelineSegments(points);
  assert.deepEqual(plain(merged.map((point) => [point.from, point.to])), [
    [0, 2000],
    [3000, 5500],
    [9000, 11000],
  ]);
  assert.equal(points[0].to, 1000); // merging cannot mutate the live history model
  assert.equal(
    helpers.stateRows(
      [
        frame([
          { name: "t", type: "time", values: [null, "0", NaN] },
          number([1, 1, 1]),
        ]),
      ],
      0,
      10,
      describe,
    ).rows[0].points.length,
    0,
  );
});

test("state labels can preserve strings and booleans; query clipping and work budgets remain bounded", () => {
  const result = helpers.stateRows(
    [
      frame([
        { name: "Time", type: "time", values: [0, 10, 20] },
        { name: "state", type: "string", values: ["active", null, "failed"] },
        { name: "ready", type: "boolean", values: [false, true, true] },
      ]),
    ],
    5,
    25,
    describe,
  );
  assert.equal(result.rows.length, 2);
  assert.deepEqual(plain(result.rows[0].points.map((point) => point.value)), [
    "active",
    "failed",
  ]);
  assert.equal(result.rows[0].points[0].from, 5);
  assert.equal(result.rows[0].points[1].to, 25);
  assert.equal(result.rows[1].points[0].value, false);
  const many = helpers.stateRows(
    [
      frame([
        {
          name: "Time",
          type: "time",
          values: Array.from({ length: 200010 }, (_, index) => index),
        },
        number(Array(200010).fill(1)),
      ]),
    ],
    0,
    200010,
    describe,
  );
  assert.equal(many.scanned, 200000);
  assert.equal(many.omitted, true);
});

test("category colors remain stable when series order changes", () => {
  assert.equal(
    helpers.colorForKey("server-1"),
    helpers.colorForKey("server-1"),
  );
  assert.match(helpers.colorForKey("服务-10002"), /^#[a-f0-9]{6}$/);
});

test("histograms separate raw distributions from classic cumulative buckets across frames", () => {
  const raw = helpers.histogramBins([
    frame([number([0, 1, 2, 3, null, "4", false, NaN])]),
  ]);
  assert.equal(raw.invalid, false);
  assert.equal(
    raw.bins.reduce((sum, bin) => sum + bin.value, 0),
    4,
  );
  const classic = helpers.histogramBins(
    [2, 5, 8].map((value, index) =>
      frame([
        {
          ...number([value]),
          labels: { le: ["0.1", "1", "+Inf"][index], serverId: "same" },
        },
      ]),
    ),
  );
  assert.deepEqual(plain(classic.bins.map((bin) => bin.value)), [2, 3, 3]);
  const broken = helpers.histogramBins([
    frame([
      { ...number([5]), labels: { le: "1" } },
      { ...number([2]), labels: { le: "2" } },
    ]),
  ]);
  assert.equal(broken.invalid, true);
  assert.equal(broken.bins.length, 0);
  const table = helpers.histogramBins([
    frame([
      { name: "bin", type: "string", values: ["fast", "slow", "missing"] },
      number([0, 9, null]),
    ]),
  ]);
  assert.deepEqual(plain(table.bins), [
    { label: "fast", value: 0 },
    { label: "slow", value: 9 },
  ]);
  assert.equal(
    helpers.histogramBins([
      frame([{ name: "bin", type: "string", values: ["bad"] }, number([-1])]),
    ]).invalid,
    true,
  );
  assert.equal(
    helpers.histogramBins([
      frame([{ name: "bin", type: "string", values: ["count"] }, number([1])]),
      frame([number([233])]),
    ]).invalid,
    true,
  );
});

test("availability mapping is recognized exactly and custom mappings remain distinct", () => {
  const helper = {};
  vm.runInNewContext(
    ts.transpileModule(
      fs.readFileSync("src/features/native-panel-states.ts", "utf8"),
      {
        compilerOptions: {
          module: ts.ModuleKind.CommonJS,
          target: ts.ScriptTarget.ES2022,
        },
      },
    ).outputText,
    { exports: helper },
  );
  assert.equal(helper.nativeStateMapping(), "");
  const panel = {
    config: {
      fieldConfig: {
        defaults: {
          mappings: [
            {
              type: "value",
              options: {
                0: { text: "Unavailable", color: "red" },
                1: { text: "Available", color: "green" },
              },
            },
          ],
        },
      },
    },
  };
  assert.equal(helper.nativeStateMapping(panel), "availability");
  panel.config.fieldConfig.defaults.mappings[0].options["1"].text = "Custom";
  assert.equal(helper.nativeStateMapping(panel), "custom");
});
