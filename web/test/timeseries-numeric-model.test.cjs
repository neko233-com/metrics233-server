const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const ts = require("typescript");
function load(file, requireModule = () => { throw new Error("Unexpected import"); }, extra = {}) {
  const exports = {};
  vm.runInNewContext(ts.transpileModule(fs.readFileSync(file, "utf8"), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText, { exports, require: requireModule, Float64Array, Uint32Array, Uint8Array, DOMException, performance, ...extra });
  return exports;
}
const numeric = load("src/render/timeseries/numeric-model.ts");
const presentation = load("src/render/series-presentation.ts");
const risk = load("src/render/timeseries/risk-thresholds.ts", name=>{
  if(name==="@grafana/data") return {createTheme:()=>({visualization:{getColorByName:name=>({green:"#39d99c",yellow:"#f4cd51",red:"#f2495c","dark-red":"#c4162a"}[name]||name)}})};
  throw new Error("Unexpected risk import: "+name);
});
const model = load("src/render/timeseries/plot-model.ts", (name) => {
  if (name === "./risk-thresholds") return risk;
  if (name === "./numeric-model") return numeric;
  if (name === "../series-presentation") return presentation;
  if (name === "../../i18n") return { getLocale: () => "en" };
  if (name === "../frame-budget") return { nextFrame: async () => {} };
  if (name === "@grafana/data") return { createTheme: () => ({}), getFieldDisplayName: (field, frame) => {
    assert.ok(frame.fields.includes(field), "metadata field identity must belong to its cloned frame");
    const alias = field.config.displayName || field.config.displayNameFromDS;
    // The pinned SDK appends this suffix to configured aliases in comparison
    // frames too; returning the alias alone misses real SQL-frame behavior.
    return alias ? alias + (frame.meta?.timeCompare?.isTimeShiftQuery ? " (comparison)" : "") : field.name;
  },
    getFieldSeriesColor: field => ({ color: field.config.color?.mode === "fixed" ? field.config.color.fixedColor : ["#5794f2", "#73bf69"][field.state?.seriesIndex % 2 || 0] }) };
  throw new Error("Unexpected import: " + name);
});
const source = (times, ...columns) => ({ times: Float64Array.from(times), columns: columns.map((values) => Float64Array.from(values)) });

test("separate query frames receive distinct classic colors while authored fixed colors and source state survive", () => {
  const frames = ["A", "B"].map(refId => ({ refId, fields: [{ name: "Time", type: "time", values: [1000, 2000], config: {} }, { name: "Value", type: "number", values: [1, 2], config: { color: { mode: "palette-classic" } }, state: { seriesIndex: 0 } }] }));
  const plot = model.describePlot(frames);
  assert.notEqual(plot.series[0].color, plot.series[1].color);
  assert.equal(frames[1].fields[1].state.seriesIndex, 0);
  for (const frame of frames) frame.fields[1].config.color = { mode: "fixed", fixedColor: "#f00" };
  assert.deepEqual(Array.from(model.describePlot(frames).series, item => item.color), ["#f00", "#f00"]);
});
test("risk thresholds preserve exact boundary colors and require a known percentage range", () => {
  const field={config:{color:{mode:"thresholds"},thresholds:{mode:"absolute",steps:[{value:null,color:"green"},{value:85,color:"yellow"},{value:95,color:"red"}]}}};
  assert.equal(risk.riskColor(field,84.99),"#39d99c");
  assert.equal(risk.riskColor(field,85),"#f4cd51");
  assert.equal(risk.riskColor(field,95),"#f2495c");
  assert.equal(risk.riskColor(field,null),undefined);
  assert.equal(risk.riskColor(field,NaN),undefined);
  field.config.thresholds.steps[2].color="dark-red";
  assert.equal(risk.riskColor(field,95),"#c4162a");
  field.config.thresholds.steps[2].color="not-a-color";
  assert.equal(risk.riskColor(field,95),"#85aaff");
  field.config.thresholds.steps[2].color="red";
  field.config.thresholds.mode="percentage";
  assert.equal(risk.riskSteps(field).length,0);
  field.config.min=100;field.config.max=200;
  assert.equal(risk.riskColor(field,184),"#39d99c");
  assert.equal(risk.riskColor(field,185),"#f4cd51");
});
test("heat strokes and warning lines use historical value coordinates without nonfinite canvas geometry", () => {
  const field={config:{color:{mode:"thresholds"},thresholds:{mode:"absolute",steps:[{value:null,color:"green"},{value:85,color:"yellow"},{value:95,color:"red"}]}}};
  const stops=[],rects=[],lines=[];
  const gradient={addColorStop:(offset,color)=>{assert.ok(offset>=0&&offset<=1);stops.push([offset,color]);}};
  const chart={bbox:{top:10,left:20,width:300,height:100},valToPos:value=>110-value,posToVal:pos=>110-pos,
    ctx:{createLinearGradient:()=>gradient,save(){},restore(){},beginPath(){},rect(){},clip(){},fillRect:(...values)=>{assert.ok(values.every(Number.isFinite));rects.push(values);},setLineDash(){},moveTo:(...values)=>lines.push(values),lineTo:(...values)=>lines.push(values),stroke(){}}};
  assert.equal(risk.riskStroke(chart,field,"y","#aaa"),gradient);
  assert.equal(stops[0][1],"#f2495c");assert.equal(stops.at(-1)[1],"#39d99c");
  risk.drawRiskBands(chart,field,"y");assert.equal(rects.length,3);
  risk.drawRiskBands(chart,field,"y",true);assert.equal(lines.length,4);
  assert.ok(lines.flat().every(Number.isFinite));
});
const settings = (count, stack = "none", axis = "y") => Array.from({ length: count }, () => ({ axis, stack, stackGroup: "A" }));
const request = (sources, series = settings(sources.reduce((sum, value) => sum + value.columns.length, 0)), targetRows = 1000,
  start = 0, end = 1e15) => ({ sources, series, targetRows, start, end });
const values = (result) => Array.from(result.columns, (column) => Array.from(column));
const plain = (value) => JSON.parse(JSON.stringify(value));

test("100k rows retain narrow spikes, troughs, endpoints and gap boundaries in every series", () => {
  const count = 100000;
  const x = Float64Array.from({ length: count }, (_, row) => row);
  const a = new Float64Array(count), b = new Float64Array(count);
  a[54321] = 999; a[54322] = -888;
  b[32123] = 777; b[32124] = -666;
  a.fill(NaN, 70000, 71000); b.fill(NaN, 12000, 12003);
  const result = numeric.prepareNumericModel(request([{ times: x, columns: [a, b] }], settings(2), 512));
  const selected = Array.from(result.columns[0]);
  for (const row of [0, 99999, 54321, 54322, 32123, 32124, 69999, 70000, 70999, 71000, 11999, 12000, 12002, 12003])
    assert.ok(selected.includes(row), `sample ${row} retained`);
  assert.ok(selected.length <= 512 + 8);
  assert.equal(result.inputRows, count);
  assert.equal(result.alignedRows, count);
  assert.equal(Math.max(...result.columns[1].filter(Number.isFinite)), 999);
  assert.equal(Math.min(...result.columns[2].filter(Number.isFinite)), -666);
});

test("unaligned, unsorted duplicate times keep the latest finite value and missing cells", () => {
  const result = numeric.prepareNumericModel(request([
    source([3, 1, 2, 2, NaN], [30, 10, 20, 22, 999]), source([2, 4, 2], [200, 400, NaN]),
  ]));
  assert.deepEqual(values(result), [[1, 2, 3, 4], [10, 22, 30, NaN], [NaN, 200, NaN, 400]]);
});

test("the extrema union shares one pixel budget across twenty series", () => {
  const count = 100000, budget = 1024;
  const x = Float64Array.from({ length: count }, (_, row) => row);
  const columns = Array.from({ length: 20 }, (_, series) => Float64Array.from(x,
    (row) => Math.sin(row / (37 + series))));
  const result = numeric.prepareNumericModel(request([{ times: x, columns }], settings(20), budget));
  assert.ok(result.columns[0].length <= budget, `${result.columns[0].length} exceeds shared ${budget} row budget`);
  assert.equal(result.columns.length, 21);
});

test("stacking runs before sampling and retains gaps, axes and independent groups", () => {
  const series = [...settings(2, "percent"), ...settings(1, "normal", "y2")];
  const result = numeric.prepareNumericModel(request([
    source([1, 2, 3], [20, NaN, -1], [80, 10, 0], [500, 600, 700]),
  ], series));
  assert.deepEqual(values(result), [[1, 2, 3], [20, NaN, 0], [100, 100, 0], [500, 600, 700]]);
});

test("viewport sampling includes neighbouring points and retains the full model legend last value", () => {
  const result = numeric.prepareNumericModel(request([source([0, 10, 20, 30, 40, 50], [0, 1, 2, 3, 4, 99])], settings(1), 100, 15, 35));
  assert.deepEqual(values(result), [[10, 20, 30, 40], [1, 2, 3, 4]]);
  assert.deepEqual(Array.from(result.lastValues), [99]);
});

test("many gaps exceed the soft pixel target rather than join disconnected samples", () => {
  const x = Float64Array.from({ length: 1000 }, (_, row) => row);
  const y = Float64Array.from(x, (row) => row % 2 ? NaN : row);
  const result = numeric.prepareNumericModel(request([{ times: x, columns: [y] }], settings(1), 100));
  assert.equal(result.columns[0].length, 1000);
  assert.ok(Number.isNaN(result.columns[1][1]));
});

test("empty frames and all-missing values stay valid", () => {
  assert.deepEqual(values(numeric.prepareNumericModel(request([]))), [[]]);
  const result = numeric.prepareNumericModel(request([source([1, 2], [NaN, NaN])]));
  assert.deepEqual(values(result), [[1, 2], [NaN, NaN]]);
  assert.ok(Number.isNaN(result.lastValues[0]));
});

test("packing preserves source DataFrames, distinguishes null from zero and aligns comparisons", async () => {
  const frames = [{ refId: "A", length: 3, meta: { timeCompare: { isTimeShiftQuery: true, diffMs: -1000 } },
    fields: [{ name: "Time", type: "time", values: [0, 1000, null], config: {} },
      { name: "Value", type: "number", values: [0, null, 5], config: { custom: { stacking: { mode: "percent" } } } }] }];
  const before = JSON.stringify(frames);
  const spec = model.describePlot(frames);
  const packed = model.packPlot(spec);
  const sliced = await model.packPlotInSlices(spec, new AbortController().signal);
  assert.deepEqual(packed, sliced);
  assert.deepEqual(Array.from(packed[0].times), [1000, 2000, NaN]);
  assert.deepEqual(Array.from(packed[0].columns[0]), [0, NaN, 5]);
  const completed = model.preparePlotSync(spec, { targetRows: 100, start: 0, end: 3000 });
  assert.equal(completed.series[0].name, "Value (Comparison)");
  assert.equal(completed.series[0].dashed, true);
  assert.deepEqual(plain(completed.data), [[1000, 2000], [0, null]]);
  assert.equal(JSON.stringify(frames), before);
  const controller = new AbortController(); controller.abort();
  await assert.rejects(model.packPlotInSlices(spec, controller.signal), { name: "AbortError" });
});

test("plot names localize metrics and preserve custom comparison aliases with locale-independent identities", () => {
  const frame = (refId, comparison = false) => ({ refId, length: 2,
    meta: comparison ? { timeCompare: { isTimeShiftQuery: true, diffMs: -1000 } } : {},
    fields: [{ name: "Time", type: "time", values: [1000, 2000], config: {} },
      { name: "Value", type: "number", values: [10, 20], config: {},
        labels: { __name__: "metricspanel_uptime_seconds", instance: "host-a", job: "self" } },
      { name: "Value", type: "number", values: [1, 2], config: { displayNameFromDS: "My exact alias" },
        labels: { __name__: "custom_total", instance: "host-a" } }] });
  const frames = [frame("A"), frame("A-compare", true)];
  const before = JSON.stringify(frames);
  const en = model.describePlot(frames, "en"), zh = model.describePlot(frames, "zh");
  assert.deepEqual(plain(en.series.map(item => item.id)), plain(zh.series.map(item => item.id)));
  assert.equal(en.series[0].name, "Monitoring service uptime");
  assert.equal(zh.series[0].name, "监控服务运行时长");
  assert.equal(zh.series[1].name, "My exact alias");
  assert.equal(zh.series[2].name, "监控服务运行时长 (比较)");
  assert.equal(zh.series[3].name, "My exact alias (比较)");
  assert.equal(zh.series[0].rawName, 'metricspanel_uptime_seconds{instance="host-a",job="self"}');
  assert.match(zh.series[0].description, /重启/);
  assert.deepEqual(plain(zh.series[0].field.labels), frames[0].fields[1].labels);
  assert.notEqual(en.signature, zh.signature, "labels redraw on locale changes while identities remain stable");
  assert.equal(JSON.stringify(frames), before);
});

test("plot names distinguish actual hosts and preserve custom field containers and duplicate identities", () => {
  const frames = ["host-a", "host-b", "host-b"].map(instance => ({ refId: "A", length: 1,
    fields: [{ name: "Time", type: "time", values: [1000], config: {} },
      { name: "Value", type: "number", values: [1], config: { displayName: "My target" },
        labels: { __name__: "up", instance, job: "self" }, state: { displayName: "old SDK value" } }] }));
  const before = JSON.stringify(frames), spec = model.describePlot(frames, "zh");
  assert.equal(new Set(spec.series.map(item => item.name)).size, 3);
  assert.equal(new Set(spec.series.map(item => item.id)).size, 3);
  assert.ok(spec.series.every(item => item.name.startsWith("My target · 实例=")));
  assert.equal(JSON.stringify(frames), before);
});

test("SDK automatic comparison suffix localizes while the author's alias body stays exact", () => {
  const frame = (refId, comparison, config) => ({ refId, length: 1,
    meta: { timeCompare: { isTimeShiftQuery: comparison, diffMs: -1000 } },
    fields: [{ name: "Time", type: "time", values: [1000], config: {} },
      { name: "__value__", type: "number", values: [3], config }] });
  const frames = [
    frame("A", false, { displayNameFromDS: "SQL business" }),
    frame("A-compare", true, { displayNameFromDS: "SQL business" }),
    frame("B", false, { displayName: "Author (comparison)" }),
    frame("B-compare", true, { displayName: "Author (comparison)" }),
    frame("C-compare", true, { displayName: "Manual alias", displayNameFromDS: "SQL alias" }),
  ];
  const before = JSON.stringify(frames), zh = model.describePlot(frames, "zh"), en = model.describePlot(frames, "en");
  assert.deepEqual(plain(zh.series.map(item => item.name)), [
    "SQL business", "SQL business (比较)", "Author (comparison)",
    "Author (comparison) (比较)", "Manual alias (比较)",
  ]);
  assert.equal(en.series[1].name, "SQL business (Comparison)");
  assert.equal(en.series[3].name, "Author (comparison) (Comparison)");
  assert.deepEqual(plain(zh.series.map(item => item.id)), plain(en.series.map(item => item.id)));
  assert.equal(JSON.stringify(frames), before);
});

test("aggregate Value fields use the panel title as a label without claiming original counter semantics", () => {
  const frames = [{ refId: "A", length: 1,
    fields: [{ name: "Time", type: "time", values: [1000], config: {} },
      { name: "Value", type: "number", values: [32], labels: { job: "host" }, config: {} }] }];
  const en = model.describePlot(frames, "en", "Host CPU utilization");
  const zh = model.describePlot(frames, "zh", "主机 CPU 使用率");
  assert.equal(en.series[0].name, "Host CPU utilization");
  assert.equal(zh.series[0].name, "主机 CPU 使用率");
  assert.equal(en.series[0].id, zh.series[0].id);
  assert.match(zh.series[0].description, /暂无/);
  assert.doesNotMatch(zh.series[0].description, /累计/);
  // Even an unfortunately named panel is an alias, never a raw metric hint.
  const misleadingTitle = model.describePlot(frames, "zh", "node_cpu_seconds_total");
  assert.equal(misleadingTitle.series[0].name, "node_cpu_seconds_total");
  assert.match(misleadingTitle.series[0].description, /暂无/);
  frames[0].fields[1].name = "Weighted latency";
  assert.equal(model.describePlot(frames, "zh", "Other title").series[0].name, "Weighted latency");
});

test("Worker protocol transfers owned buffers in both directions and matches synchronous output", () => {
  const originalTimes = [1, 2, 3], originalValues = [8, NaN, 9];
  const packet = request([source(originalTimes, originalValues)]);
  const transferred = packet.sources.flatMap((entry) => [entry.times.buffer, ...entry.columns.map((column) => column.buffer)]);
  const workerInput = structuredClone(packet, { transfer: transferred });
  transferred.forEach((buffer) => assert.equal(buffer.byteLength, 0));
  let response;
  const worker = { postMessage: (message, options) => {
    assert.equal(options.transfer.length, 2);
    response = structuredClone(message, { transfer: Array.from(options.transfer) });
  } };
  load("src/render/timeseries/numeric-model.worker.ts", () => numeric, { self: worker });
  worker.onmessage({ data: workerInput });
  assert.deepEqual(values(response.result), values(numeric.prepareNumericModel(request([source(originalTimes, originalValues)]))));
  assert.deepEqual(originalValues, [8, NaN, 9]);
});
