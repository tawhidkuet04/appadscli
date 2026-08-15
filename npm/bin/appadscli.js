#!/usr/bin/env node
// Thin launcher: ensures the platform binary exists (downloading it if the
// postinstall was skipped, e.g. --ignore-scripts), then hands over entirely.
"use strict";

const fs = require("fs");
const { spawnSync } = require("child_process");
const { install, binaryPath } = require("../scripts/install.js");

async function main() {
  let bin = binaryPath();
  if (!fs.existsSync(bin)) {
    console.error("appadscli binary not present, downloading…");
    bin = await install();
  }
  const res = spawnSync(bin, process.argv.slice(2), { stdio: "inherit" });
  if (res.error) throw res.error;
  process.exit(res.status ?? 1);
}

main().catch((err) => {
  console.error(`appadscli: ${err.message}`);
  process.exit(1);
});
