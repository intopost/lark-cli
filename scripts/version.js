#!/usr/bin/env node

const fs = require("fs");
const path = require("path");

const rootDir = path.join(__dirname, "..");
const goModPath = path.join(rootDir, "go.mod");
const packagePath = path.join(rootDir, "package.json");

function getVersion() {
  const goMod = fs.readFileSync(goModPath, "utf8");
  const match = goMod.match(/^\s*(?:require\s+)?github\.com\/larksuite\/cli\s+v([^\s]+)\s*$/m);
  if (!match) {
    throw new Error("无法从 go.mod 读取 github.com/larksuite/cli 版本");
  }

  const pkg = JSON.parse(fs.readFileSync(packagePath, "utf8"));
  const revision = pkg.xybotRevision;
  if (!Number.isInteger(revision) || revision < 1) {
    throw new Error("package.json 的 xybotRevision 必须是大于 0 的整数");
  }
  return `${match[1]}-xybot.${revision}`;
}

function main() {
  const expected = getVersion();
  const pkg = JSON.parse(fs.readFileSync(packagePath, "utf8"));

  if (process.argv.includes("--write")) {
    pkg.version = expected;
    fs.writeFileSync(packagePath, `${JSON.stringify(pkg, null, 2)}\n`);
    console.log(`版本号已同步为 ${expected}`);
    return;
  }

  if (process.argv.includes("--check")) {
    if (pkg.version !== expected) {
      throw new Error(`版本号不一致: package.json=${pkg.version}, 期望=${expected}；请运行 npm run version:sync`);
    }
    return;
  }

  console.log(expected);
}

if (require.main === module) {
  try {
    main();
  } catch (error) {
    console.error(error.message || error);
    process.exit(1);
  }
}

module.exports = { getVersion };
