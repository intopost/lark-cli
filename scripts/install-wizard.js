#!/usr/bin/env node

const { execFileSync } = require("child_process");

const pkg = require("../package.json");

const PKG = pkg.name;
const VERSION = pkg.version;
const INSTALL_SPEC = process.env.LARK_CLI_NPM_SPEC || `${PKG}@${VERSION}`;
const npmCmd = process.platform === "win32" ? "npm.cmd" : "npm";

function run(cmd, args, options = {}) {
  return execFileSync(cmd, args, {
    stdio: "inherit",
    ...options,
  });
}

function runQuiet(cmd, args, options = {}) {
  return execFileSync(cmd, args, {
    stdio: ["ignore", "pipe", "pipe"],
    ...options,
  }).toString();
}

function getInstalledVersion() {
  try {
    const output = runQuiet(npmCmd, ["list", "-g", PKG, "--depth=0"]);
    const match = output.match(/@(\d+\.\d+\.\d+[^\s]*)/);
    return match ? match[1] : null;
  } catch (_) {
    return null;
  }
}

function getOldCliVersion() {
  try {
    const binName = process.platform === "win32" ? "lark-cli.cmd" : "lark-cli";
    const output = runQuiet(binName, ["--version"]);
    return output.trim();
  } catch (_) {
    return null;
  }
}

function installGlobally() {
  const installedVersion = getInstalledVersion();
  if (installedVersion === VERSION) {
    console.log(`${PKG} 已全局安装 (${VERSION})`);
    return;
  }

  const oldCliVersion = getOldCliVersion();
  if (oldCliVersion) {
    console.log(`正在升级 lark-cli (当前版本: ${oldCliVersion} -> 新版本: @intopost/lark-cli@${VERSION}) ...`);
  } else {
    console.log(`正在全局安装 ${INSTALL_SPEC} ...`);
  }

  // 加上 --force 以覆盖可能存在的官方 lark-cli 软链接，避免 EEXIST 错误
  run(npmCmd, ["install", "-g", INSTALL_SPEC, "--force"]);
  console.log("安装完成，后续可直接运行 `lark-cli`");
}

if (require.main === module) {
  try {
    installGlobally();
  } catch (error) {
    console.error(error.message || error);
    process.exit(1);
  }
}

module.exports = {
  installGlobally,
};
