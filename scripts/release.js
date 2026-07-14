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

const changes = read("git", ["status", "--porcelain"])
  .split("\n")
  .filter(Boolean);
if (changes.some((line) => line.slice(3) !== "package.json")) {
  console.error("除 package.json 外存在未提交改动，请先提交后再发布");
  process.exit(1);
}

const branch = read("git", ["branch", "--show-current"]);
if (!branch) {
  console.error("当前处于 detached HEAD，无法自动发布");
  process.exit(1);
}

const version = getVersion();
const tag = `v${version}`;
const local = spawnSync("git", ["rev-parse", "--verify", "--quiet", `refs/tags/${tag}`], { cwd: root });
const remote = spawnSync("git", ["ls-remote", "--exit-code", "--tags", "origin", `refs/tags/${tag}`], {
  cwd: root,
  stdio: "ignore",
});

if (local.status === 0 || remote.status === 0) {
  console.error(`Tag ${tag} 已存在，请先增加 package.json 的 xybotRevision`);
  process.exit(1);
}
if (remote.status !== 2) {
  console.error("无法确认远程 Tag 状态，请检查网络和 origin 权限");
  process.exit(1);
}

console.log(`准备发布 ${tag}（分支: ${branch}）`);
run("go", ["test", "./..."]);

if (dry) {
  console.log("预检通过；未修改文件、创建 Tag 或推送代码");
  process.exit(0);
}

const file = path.join(root, "package.json");
const pkg = JSON.parse(fs.readFileSync(file, "utf8"));
pkg.version = version;
fs.writeFileSync(file, `${JSON.stringify(pkg, null, 2)}\n`);
const changed = spawnSync("git", ["diff", "--quiet", "HEAD", "--", "package.json"], { cwd: root });
if (changed.status === 1) {
  run("git", ["add", "package.json"]);
  run("git", ["commit", "-m", `release: ${tag}`]);
}

run("node", ["scripts/version.js", "--check"]);
run("git", ["tag", "-a", tag, "-m", tag]);
run("git", ["push", "origin", branch]);
run("git", ["push", "origin", tag]);
console.log(`${tag} 已推送，GitHub Actions 将继续构建并发布 npm 包`);
