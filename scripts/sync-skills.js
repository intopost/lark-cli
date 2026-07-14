#!/usr/bin/env node

const fs = require("fs");
const path = require("path");
const { execFileSync } = require("child_process");

const root = path.join(__dirname, "..");
const args = process.argv.slice(2);
const dry = args.includes("--dry-run");
const input = args.find((arg) => arg !== "--dry-run") || process.env.LARK_CLI_SKILLS_TARGET;

if (!input) {
  console.error("用法: npm run skills:sync -- [--dry-run] <目标目录>");
  process.exit(1);
}

const mod = execFileSync("go", ["list", "-m", "-f", "{{.Dir}}", "github.com/larksuite/cli"], {
  cwd: root,
  encoding: "utf8",
}).trim();
const source = path.join(mod, "skills");
const target = path.resolve(input);

if (!fs.existsSync(source) || !fs.statSync(source).isDirectory()) {
  console.error(`未找到官方 Skills 目录: ${source}`);
  process.exit(1);
}

if (target === path.parse(target).root) {
  console.error("拒绝同步到文件系统根目录");
  process.exit(1);
}

fs.mkdirSync(target, { recursive: true });
execFileSync("rsync", ["-av", "--delete", "--chmod=u+rwX", ...(dry ? ["--dry-run"] : []), `${source}/`, `${target}/`], {
  stdio: "inherit",
});

console.log(`${dry ? "预览" : "同步"}完成: ${source} -> ${target}`);
