#!/usr/bin/env node

const fs = require("fs");
const path = require("path");
const { execFileSync } = require("child_process");

const { install } = require("./install.js");
const { installGlobally } = require("./install-wizard.js");

const binaryName = "lark-cli" + (process.platform === "win32" ? ".exe" : "");
const binaryPath = path.join(__dirname, "..", "bin", binaryName);

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
