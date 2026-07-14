#!/usr/bin/env node

const fs = require("fs");
const path = require("path");
const { execFileSync, spawnSync } = require("child_process");
const { getVersion } = require("./version");

const root = path.join(__dirname, "..");
const dry = process.argv.includes("--dry-run");

function run(cmd, args, opts = {}) {
  return execFileSync(cmd, args, { cwd: root, stdio: "inherit", ...opts });
}

function read(cmd, args) {
  return execFileSync(cmd, args, { cwd: root, encoding: "utf8" }).trim();
}

const branch = read("git", ["branch", "--show-current"]);
if (!branch) {
  console.error("当前处于 detached HEAD，无法自动发布");
  process.exit(1);
}

const file = path.join(root, "package.json");
const pkg = JSON.parse(fs.readFileSync(file, "utf8"));
const base = getVersion().replace(/-xybot\.\d+$/, "");

function exists(tag) {
  if (spawnSync("git", ["rev-parse", "--verify", "--quiet", `refs/tags/${tag}`], { cwd: root }).status === 0) {
    return true;
  }
  const result = spawnSync("git", ["ls-remote", "--exit-code", "--tags", "origin", `refs/tags/${tag}`], {
    cwd: root,
    stdio: "ignore",
  });
  if (result.status === 0) return true;
  if (result.status === 2) return false;
  console.error("无法确认远程 Tag 状态，请检查网络和 origin 权限");
  process.exit(1);
}

while (exists(`v${base}-xybot.${pkg.xybotRevision}`)) pkg.xybotRevision++;
const version = `${base}-xybot.${pkg.xybotRevision}`;
const tag = `v${version}`;

console.log(`准备发布 ${tag}（分支: ${branch}）`);
run("go", ["test", "./..."]);

if (dry) {
  console.log(`预检通过；正式执行时将提交当前仓库改动并发布 ${tag}`);
  process.exit(0);
}

pkg.version = version;
fs.writeFileSync(file, `${JSON.stringify(pkg, null, 2)}\n`);
run("node", ["scripts/version.js", "--check"]);
run("git", ["add", "-A"]);
const changed = spawnSync("git", ["diff", "--cached", "--quiet"], { cwd: root });
if (changed.status === 1) {
  run("git", ["commit", "-m", `release: ${tag}`]);
}

run("git", ["tag", "-a", tag, "-m", tag]);
run("git", ["push", "origin", branch]);
run("git", ["push", "origin", tag]);
console.log(`${tag} 已推送，GitHub Actions 将继续构建并发布 npm 包`);
