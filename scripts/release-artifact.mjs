import { createHash } from "node:crypto";
import { createReadStream } from "node:fs";
import { open } from "node:fs/promises";
import { Transform, Writable } from "node:stream";
import { pipeline } from "node:stream/promises";
import { crc32, createDeflateRaw } from "node:zlib";

// Fingerprint every repository input consumed by Vite or the embedded Go build.
export function isBuildInput(file) {
  return ["go.mod", "go.sum", "README.md", "README-CN.md", "LICENSE", "web/package.json", "web/package-lock.json", "web/vite.config.ts", "web/index.html"].includes(file) ||
    /^(cmd|internal|web\/src|web\/public)\//.test(file) || /^web\/[^/]+\.go$/.test(file) || /^web\/tsconfig.*\.json$/.test(file) || /^scripts\/(build-release|release-artifact)/.test(file);
}

export async function sha256(file) {
  const hash = createHash("sha256");
  for await (const chunk of createReadStream(file)) hash.update(chunk);
  return hash.digest("hex");
}

// Ordinary ZIP32 with fixed DOS date (1980-01-01), sorted paths and Unix modes.
// Streaming keeps large static Go executables out of the release process heap.
export async function writeDeterministicZip(output, entries) {
  const names = new Set();
  for (const entry of entries) {
    if (!entry.name || entry.name.includes("\\") || entry.name.startsWith("/") ||
        entry.name.split("/").some((part) => !part || part === "." || part === "..") ||
        entry.name.includes(":")) throw new Error("Unsafe archive entry name");
    if (names.has(entry.name)) throw new Error("Duplicate archive entry name");
    names.add(entry.name);
  }
  if (entries.length > 65535) throw new Error("Release archive exceeds ZIP32 entry limit");
  const file = await open(output, "wx");
  let offset = 0;
  const central = [];
  const write = async (value) => {
    let position = 0;
    while (position < value.length) {
      const result = await file.write(value, position, value.length - position, offset);
      if (!result.bytesWritten) throw new Error("Release archive write made no progress");
      position += result.bytesWritten;
      offset += result.bytesWritten;
    }
    if (offset > 0xffffffff) throw new Error("Release archive exceeds ZIP32 size limit");
  };
  try {
    for (const entry of [...entries].sort((a, b) => a.name < b.name ? -1 : a.name > b.name ? 1 : 0)) {
      const name = Buffer.from(entry.name, "utf8");
      if (name.length > 65535) throw new Error("Archive name exceeds ZIP32 limit");
      const localOffset = offset;
      const flags = 0x808; // UTF-8 and a post-data CRC/length descriptor.
      const header = Buffer.alloc(30);
      header.writeUInt32LE(0x04034b50, 0);
      header.writeUInt16LE(20, 4);
      header.writeUInt16LE(flags, 6);
      header.writeUInt16LE(8, 8);
      header.writeUInt16LE(33, 12);
      header.writeUInt16LE(name.length, 26);
      await write(header);
      await write(name);
      let checksum = 0, bytes = 0, compressed = 0;
      await pipeline(createReadStream(entry.path), new Transform({
        transform(chunk, encoding, callback) {
          bytes += chunk.length;
          if (bytes > 0xffffffff) { callback(new Error("Release file exceeds ZIP32 size limit")); return; }
          checksum = crc32(chunk, checksum);
          callback(null, chunk);
        },
      }), createDeflateRaw({ level: 9 }), new Writable({
        write(chunk, encoding, callback) {
          compressed += chunk.length;
          write(chunk).then(() => callback(), callback);
        },
      }));
      const descriptor = Buffer.alloc(16);
      descriptor.writeUInt32LE(0x08074b50, 0);
      descriptor.writeUInt32LE(checksum, 4);
      descriptor.writeUInt32LE(compressed, 8);
      descriptor.writeUInt32LE(bytes, 12);
      await write(descriptor);
      const record = Buffer.alloc(46);
      record.writeUInt32LE(0x02014b50, 0);
      record.writeUInt16LE((3 << 8) | 20, 4);
      record.writeUInt16LE(20, 6);
      record.writeUInt16LE(flags, 8);
      record.writeUInt16LE(8, 10);
      record.writeUInt16LE(33, 14);
      record.writeUInt32LE(checksum, 16);
      record.writeUInt32LE(compressed, 20);
      record.writeUInt32LE(bytes, 24);
      record.writeUInt16LE(name.length, 28);
      record.writeUInt32LE(((entry.executable ? 0o100755 : 0o100644) << 16) >>> 0, 38);
      record.writeUInt32LE(localOffset, 42);
      central.push(record, name);
    }
    const directoryOffset = offset;
    for (const record of central) await write(record);
    const directorySize = offset - directoryOffset;
    const end = Buffer.alloc(22);
    end.writeUInt32LE(0x06054b50, 0);
    end.writeUInt16LE(entries.length, 8);
    end.writeUInt16LE(entries.length, 10);
    end.writeUInt32LE(directorySize, 12);
    end.writeUInt32LE(directoryOffset, 16);
    await write(end);
    await file.sync();
  } finally { await file.close(); }
}
