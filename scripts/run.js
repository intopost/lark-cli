#!/usr/bin/env bun

const fs = require("fs");
const path = require("path");
const { execFileSync } = require("child_process");

// bun does not resolve __dirname through symlinks; realpathSync gives the actual script dir
const scriptDir = path.dirname(fs.realpathSync(__filename));

const { install } = require(path.join(scriptDir, "install.js"));
const { installGlobally } = require(path.join(scriptDir, "install-wizard.js"));

const binaryName = "lark-cli" + (process.platform === "win32" ? ".exe" : "");
const binaryPath = path.join(scriptDir, "..", "bin", binaryName);

async function ensureBinary() {
  if (fs.existsSync(binaryPath)) {
    return;
  }
  await install();
}

async function main() {
  const args = process.argv.slice(2);

  if (args[0] === "install") {
    installGlobally();
    return;
  }

  await ensureBinary();

  try {
    execFileSync(binaryPath, args, { stdio: "inherit" });
  } catch (error) {
    process.exit(error.status || 1);
  }
}

main().catch((error) => {
  console.error(error.message || error);
  process.exit(1);
});
