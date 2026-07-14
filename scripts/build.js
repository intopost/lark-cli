#!/usr/bin/env node

const path = require("path");
const { execFileSync } = require("child_process");
const { getVersion } = require("./version");

const version = getVersion();
const goPath = execFileSync("go", ["env", "GOPATH"], { encoding: "utf8" }).trim();
const binary = path.join(goPath, "bin", process.platform === "win32" ? "lark-cli.exe" : "lark-cli");
const ldflags = [
  `-X github.com/larksuite/cli/internal/build.Version=${version}`,
].join(" ");

execFileSync("go", ["build", "-ldflags", ldflags, "-o", binary, "."], {
  cwd: path.join(__dirname, ".."),
  stdio: "inherit",
});
console.log(`已构建 ${binary} (${version})`);
