#!/usr/bin/env bun

const fs = require("fs");
const path = require("path");
const { execFileSync } = require("child_process");

// bun global install copies this file to ~/.bun/bin/ instead of symlinking,
// so __dirname points to the bin dir and relative requires break.
// Fall back to the known bun global packages path when install.js isn't alongside us.
function findScriptDir() {
  if (fs.existsSync(path.join(__dirname, "install.js"))) {
    return __dirname;
  }
  const home = process.env.HOME || process.env.USERPROFILE || "/root";
  const bunGlobal = path.join(home, ".bun", "install", "global", "node_modules", "@intopost", "lark-cli", "scripts");
  if (fs.existsSync(path.join(bunGlobal, "install.js"))) {
    return bunGlobal;
  }
  throw new Error("Cannot find lark-cli package scripts. Try reinstalling: bun install -g @intopost/lark-cli");
}

const scriptDir = findScriptDir();

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
