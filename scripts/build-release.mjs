import { createHash } from "node:crypto";
import { execFileSync, spawn } from "node:child_process";
import { lstat, mkdir, mkdtemp, readdir, rename, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { isBuildInput, sha256, writeDeterministicZip } from "./release-artifact.mjs";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const defaultTargets = ["windows/amd64", "linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64"];
const options = { version: "0.1.1", targets: defaultTargets, output: "" };
const args = process.argv.slice(2);
for (let index = 0; index < args.length; index++) {
  const key = args[index];
  if (["--help", "-h"].includes(key)) {
    console.log("node scripts/build-release.mjs [--version SEMVER] [--targets windows/amd64,linux/amd64,...] [--output NEW_DIRECTORY]");
    process.exit(0);
  }
  if (!["--version", "--targets", "--output"].includes(key) || !args[index + 1]) throw new Error("Unknown or missing release option");
  const value = args[++index];
  options[key.slice(2)] = key === "--targets" ? value.split(",") : value;
}
if (!/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/.test(options.version)) throw new Error("Version must be a path-safe semantic version");
if (!options.targets.length || new Set(options.targets).size !== options.targets.length || options.targets.some((target) => !defaultTargets.includes(target))) throw new Error("Targets must be unique supported release targets");
const output = path.resolve(options.output || path.join(root, "dist", options.version));
if (output === root || path.dirname(output) === output) throw new Error("Release output must be a dedicated new directory");
const outputRelative = path.relative(root, output).replaceAll(path.sep, "/").toLowerCase();
if (/^(cmd|internal|web|scripts|data|bin|\.git)(\/|$)/.test(outputRelative)) throw new Error("Release output cannot be inside source or runtime data directories");

function metadata(command, values) { return execFileSync(command, values, { cwd: root, encoding: "utf8", windowsHide: true }).trim(); }
async function run(command, values, cwd = root, env = process.env) {
  await new Promise((resolve, reject) => {
    const child = spawn(command, values, { cwd, env, stdio: "inherit", windowsHide: true });
    child.once("error", reject);
    child.once("exit", (code, signal) => code === 0 ? resolve() : reject(new Error(`${command} failed (${signal || code})`)));
  });
}
async function npmTask(task) {
  // Only these constant commands cross the Windows command-shell boundary.
  const commands = { install: "npm ci", build: "npm run build" };
  if (process.platform === "win32") await run("cmd.exe", ["/d", "/s", "/c", commands[task]], path.join(root, "web"));
  else await run("npm", task === "install" ? ["ci"] : ["run", "build"], path.join(root, "web"));
}
async function sourceHash() {
  const files = execFileSync("git", ["ls-files", "-z", "--cached", "--others", "--exclude-standard"], { cwd: root }).toString("utf8").split("\0").filter(Boolean).filter(isBuildInput).sort();
  const hash = createHash("sha256");
  for (const file of files) {
    const absolute = path.resolve(root, file);
    if (path.relative(root, absolute).startsWith("..")) throw new Error("Build input escaped source root");
    const stat = await lstat(absolute);
    if (!stat.isFile() || stat.isSymbolicLink()) throw new Error("Release build inputs must be regular repository files");
    hash.update(file); hash.update("\0"); hash.update(await sha256(absolute)); hash.update("\n");
  }
  return hash.digest("hex");
}
async function treeHash(directory) {
  const hash = createHash("sha256");
  const visit = async (relative = "") => {
    const entries = await readdir(path.join(directory, relative), { withFileTypes: true });
    for (const entry of entries.sort((a, b) => a.name < b.name ? -1 : a.name > b.name ? 1 : 0)) {
      const name = relative ? `${relative}/${entry.name}` : entry.name;
      if (entry.isDirectory()) await visit(name);
      else if (entry.isFile()) { hash.update(name); hash.update("\0"); hash.update(await sha256(path.join(directory, name))); hash.update("\n"); }
      else throw new Error("Frontend release bundle contains a non-regular entry");
    }
  };
  await visit();
  return hash.digest("hex");
}

let staging;
try {
  try { await lstat(output); throw new Error("Release output already exists; choose a new directory (nothing was removed)"); }
  catch (error) { if (error.code !== "ENOENT") throw error; }
  const before = await sourceHash();
  const revision = metadata("git", ["rev-parse", "HEAD"]);
  const dirty = metadata("git", ["status", "--porcelain"]).length > 0;
  const compiler = metadata("go", ["version"]);
  await npmTask("install");
  await npmTask("build");
  const frontend = await treeHash(path.join(root, "web", "dist"));
  await mkdir(path.dirname(output), { recursive: true });
  staging = await mkdtemp(path.join(path.dirname(output), ".metricspanel-release-"));
  const artifacts = [];
  for (const target of options.targets) {
    const [goos, goarch] = target.split("/");
    const buildDir = await mkdtemp(path.join(staging, ".build-"));
    const executable = goos === "windows" ? "metricspanel.exe" : "metricspanel";
    const binary = path.join(buildDir, executable);
    await run("go", ["build", "-mod=readonly", "-buildvcs=false", "-tags", "webui", "-trimpath", "-ldflags", `-s -w -buildid= -X main.version=${options.version}`, "-o", binary, "./cmd/metricspanel"], root,
      { ...process.env, CGO_ENABLED: "0", GOWORK: "off", GOOS: goos, GOARCH: goarch, GOAMD64: "v1", GOARM64: "v8.0" });
    const binarySHA256 = await sha256(binary);
    const buildInfo = { formatVersion: 1, version: options.version, target, sourceRevision: revision, sourceDirty: dirty, sourceSHA256: before, frontendSHA256: frontend, compiler, node: process.version, binarySHA256 };
    const infoFile = path.join(buildDir, "BUILDINFO.json");
    await writeFile(infoFile, JSON.stringify(buildInfo, null, 2) + "\n", { flag: "wx" });
    const name = `metricspanel_${options.version}_${goos}_${goarch}.zip`;
    const archive = path.join(staging, name);
    await writeDeterministicZip(archive, [
      { name: executable, path: binary, executable: true },
      { name: "BUILDINFO.json", path: infoFile },
      { name: "README.md", path: path.join(root, "README.md") },
      { name: "README-CN.md", path: path.join(root, "README-CN.md") },
      { name: "LICENSE", path: path.join(root, "LICENSE") },
    ]);
    artifacts.push({ file: name, target, sha256: await sha256(archive), binarySHA256 });
    // This directory was created by this run, directly under its staging root.
    if (path.dirname(buildDir) !== staging || !path.basename(buildDir).startsWith(".build-")) throw new Error("Unsafe release temporary directory");
    await rm(buildDir, { recursive: true, force: true });
    console.log(`built ${name}`);
  }
  if (await sourceHash() !== before || await treeHash(path.join(root, "web", "dist")) !== frontend) throw new Error("Build inputs changed during release; no release directory was published");
  const provenance = { formatVersion: 1, version: options.version, sourceRevision: revision, sourceDirty: dirty, sourceSHA256: before, frontendSHA256: frontend, compiler, node: process.version, artifacts };
  await writeFile(path.join(staging, "PROVENANCE.json"), JSON.stringify(provenance, null, 2) + "\n", { flag: "wx" });
  const checksums = [...artifacts.map(({ file, sha256 }) => ({ file, sha256 })), { file: "PROVENANCE.json", sha256: await sha256(path.join(staging, "PROVENANCE.json")) }].sort((a, b) => a.file < b.file ? -1 : 1);
  await writeFile(path.join(staging, "SHA256SUMS"), checksums.map(({ file, sha256 }) => `${sha256}  ${file}\n`).join(""), { flag: "wx" });
  try { await lstat(output); throw new Error("Release output appeared during the build; existing files were preserved"); }
  catch (error) { if (error.code !== "ENOENT") throw error; }
  await rename(staging, output);
  staging = undefined;
  console.log(`Release artifacts: ${output}`);
} catch (error) {
  console.error(error.message);
  process.exitCode = 1;
} finally {
  if (staging && path.dirname(staging) === path.dirname(output) && path.basename(staging).startsWith(".metricspanel-release-")) await rm(staging, { recursive: true, force: true });
}
