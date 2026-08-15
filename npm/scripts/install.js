// Downloads the asacli binary for this platform from GitHub Releases,
// verifies its SHA-256 against the release's checksums.txt, and unpacks it
// into vendor/. No dependencies — Node 18+ stdlib only.
//
// Runs as the package postinstall; bin/asacli.js also invokes it lazily if
// the binary is missing (e.g. the package was installed with --ignore-scripts).
"use strict";

const fs = require("fs");
const path = require("path");
const zlib = require("zlib");
const crypto = require("crypto");

const REPO = "tawhidkuet04/asacli";
const VERSION = require(path.join(__dirname, "..", "package.json")).version;

const PLATFORMS = {
  "darwin-x64": { os: "darwin", arch: "amd64", ext: "tar.gz" },
  "darwin-arm64": { os: "darwin", arch: "arm64", ext: "tar.gz" },
  "linux-x64": { os: "linux", arch: "amd64", ext: "tar.gz" },
  "linux-arm64": { os: "linux", arch: "arm64", ext: "tar.gz" },
  "win32-x64": { os: "windows", arch: "amd64", ext: "zip" },
  "win32-arm64": { os: "windows", arch: "arm64", ext: "zip" },
};

function binaryPath() {
  const name = process.platform === "win32" ? "asacli.exe" : "asacli";
  return path.join(__dirname, "..", "vendor", name);
}

async function download(url) {
  const res = await fetch(url, { redirect: "follow" });
  if (!res.ok) {
    throw new Error(`download failed: HTTP ${res.status} for ${url}`);
  }
  return Buffer.from(await res.arrayBuffer());
}

// Minimal tar reader: yields {name, data} for regular files.
function* untar(buf) {
  let off = 0;
  while (off + 512 <= buf.length) {
    const name = buf.toString("utf8", off, off + 100).replace(/\0.*$/, "");
    if (!name) break;
    const size = parseInt(buf.toString("utf8", off + 124, off + 136).trim(), 8) || 0;
    const type = buf[off + 156];
    const data = buf.subarray(off + 512, off + 512 + size);
    if (type === 0x30 || type === 0) yield { name, data }; // '0' = regular file
    off += 512 + Math.ceil(size / 512) * 512;
  }
}

// Minimal zip reader: walks central directory, inflates one entry by name suffix.
function unzipFile(buf, suffix) {
  // Find end-of-central-directory record.
  let eocd = -1;
  for (let i = buf.length - 22; i >= 0 && i >= buf.length - 65558; i--) {
    if (buf.readUInt32LE(i) === 0x06054b50) { eocd = i; break; }
  }
  if (eocd < 0) throw new Error("zip: end of central directory not found");
  let count = buf.readUInt16LE(eocd + 10);
  let off = buf.readUInt32LE(eocd + 16);
  for (let i = 0; i < count; i++) {
    if (buf.readUInt32LE(off) !== 0x02014b50) throw new Error("zip: bad central directory");
    const method = buf.readUInt16LE(off + 10);
    const compSize = buf.readUInt32LE(off + 20);
    const nameLen = buf.readUInt16LE(off + 28);
    const extraLen = buf.readUInt16LE(off + 30);
    const commentLen = buf.readUInt16LE(off + 32);
    const localOff = buf.readUInt32LE(off + 42);
    const name = buf.toString("utf8", off + 46, off + 46 + nameLen);
    if (name.endsWith(suffix)) {
      const lNameLen = buf.readUInt16LE(localOff + 26);
      const lExtraLen = buf.readUInt16LE(localOff + 28);
      const start = localOff + 30 + lNameLen + lExtraLen;
      const raw = buf.subarray(start, start + compSize);
      return method === 0 ? Buffer.from(raw) : zlib.inflateRawSync(raw);
    }
    off += 46 + nameLen + extraLen + commentLen;
  }
  throw new Error(`zip: ${suffix} not found in archive`);
}

async function install() {
  const key = `${process.platform}-${process.arch}`;
  const plat = PLATFORMS[key];
  if (!plat) {
    throw new Error(
      `unsupported platform ${key} — install with Go instead: go install github.com/${REPO}@latest`
    );
  }
  const asset = `asacli_${VERSION}_${plat.os}_${plat.arch}.${plat.ext}`;
  const base = `https://github.com/${REPO}/releases/download/v${VERSION}`;

  const archive = await download(`${base}/${asset}`);

  // Verify against checksums.txt from the same release.
  const sums = (await download(`${base}/checksums.txt`)).toString("utf8");
  const line = sums.split("\n").find((l) => l.trim().endsWith(asset));
  if (!line) throw new Error(`checksums.txt has no entry for ${asset}`);
  const expected = line.trim().split(/\s+/)[0];
  const actual = crypto.createHash("sha256").update(archive).digest("hex");
  if (actual !== expected) {
    throw new Error(`checksum mismatch for ${asset}: expected ${expected}, got ${actual}`);
  }

  const wanted = plat.os === "windows" ? "asacli.exe" : "asacli";
  let binary = null;
  if (plat.ext === "zip") {
    binary = unzipFile(archive, wanted);
  } else {
    for (const f of untar(zlib.gunzipSync(archive))) {
      if (f.name === wanted || f.name.endsWith("/" + wanted)) { binary = Buffer.from(f.data); break; }
    }
  }
  if (!binary) throw new Error(`${wanted} not found in ${asset}`);

  const dest = binaryPath();
  fs.mkdirSync(path.dirname(dest), { recursive: true });
  fs.writeFileSync(dest, binary, { mode: 0o755 });
  return dest;
}

module.exports = { install, binaryPath };

if (require.main === module) {
  install()
    .then((dest) => console.log(`asacli ${VERSION} installed (${dest})`))
    .catch((err) => {
      console.error(`asacli install failed: ${err.message}`);
      console.error(`fallbacks: brew install tawhidkuet04/tap/asacli · go install github.com/${REPO}@latest`);
      process.exit(1);
    });
}
