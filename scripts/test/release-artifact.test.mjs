import { test } from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtemp, readFile, rm, utimes, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { isBuildInput, sha256, writeDeterministicZip } from "../release-artifact.mjs";

test("source identity includes frontend entry and Go embed sources", () => {
  for (const file of ["README-CN.md", "web/index.html", "web/embed.go", "web/embed_dev.go", "web/src/main.tsx", "web/public/favicon.svg", "internal/server/server.go", "scripts/build-release.mjs"]) {
    assert.equal(isBuildInput(file), true, file);
  }
  for (const file of ["web/dist/index.html", "web/node_modules/test/index.js", "data/metrics.db", "CODELY.md"]) {
    assert.equal(isBuildInput(file), false, file);
  }
});

test("release ZIP is deterministic and independently readable with checksums and executable modes", async () => {
  const temp = await mkdtemp(path.join(os.tmpdir(), "metricspanel-release-zip-test-"));
  try {
    const binary = path.join(temp, "binary");
    const readme = path.join(temp, "readme");
    const info = path.join(temp, "info");
    await writeFile(binary, Buffer.from(Array.from({ length: 65537 }, (_, index) => index % 251)));
    await writeFile(readme, "MetricsPanel233\n业务监控\n");
    await writeFile(info, '{"version":"0.1.1-test"}\n');
    const entries = [{ name: "metricspanel", path: binary, executable: true }, { name: "README.md", path: readme }, { name: "BUILDINFO.json", path: info }];
    const first = path.join(temp, "first.zip");
    const second = path.join(temp, "second.zip");
    await writeDeterministicZip(first, entries);
    await utimes(binary, new Date("2020-01-01"), new Date("2025-10-21"));
    await writeDeterministicZip(second, [...entries].reverse());
    assert.equal(await sha256(first), await sha256(second));
    const python = process.env.PYTHON || (process.platform === "win32" ? "python" : "python3");
    const result = execFileSync(python, ["-c", [
      "import hashlib, json, sys, zipfile",
      "with zipfile.ZipFile(sys.argv[1]) as archive:",
      " assert archive.testzip() is None",
      " assert archive.namelist() == ['BUILDINFO.json', 'README.md', 'metricspanel']",
      " assert all(i.date_time == (1980, 1, 1, 0, 0, 0) for i in archive.infolist())",
      " assert (archive.getinfo('metricspanel').external_attr >> 16) & 0o777 == 0o755",
      " assert (archive.getinfo('README.md').external_attr >> 16) & 0o777 == 0o644",
      " assert json.loads(archive.read('BUILDINFO.json'))['version'] == '0.1.1-test'",
      " assert archive.read('README.md').decode('utf8').endswith('业务监控\\n')",
      " print(hashlib.sha256(archive.read('metricspanel')).hexdigest())",
    ].join("\n"), first], { encoding: "utf8", windowsHide: true });
    assert.equal(result.trim(), await sha256(binary));
  } finally { await rm(temp, { recursive: true, force: true }); }
});

test("archive path escapes and duplicate names fail before creating an output file", async () => {
  const temp = await mkdtemp(path.join(os.tmpdir(), "metricspanel-release-zip-test-"));
  try {
    const output = path.join(temp, "unsafe.zip");
    for (const name of ["../outside", "/absolute", "C:/drive", "nested\\file", "nested/../file"]) {
      await assert.rejects(writeDeterministicZip(output, [{ name, path: "unused" }]), /Unsafe archive entry/);
    }
    await assert.rejects(writeDeterministicZip(output, [{ name: "same", path: "unused" }, { name: "same", path: "unused" }]), /Duplicate/);
    await assert.rejects(readFile(output), { code: "ENOENT" });
  } finally { await rm(temp, { recursive: true, force: true }); }
});
