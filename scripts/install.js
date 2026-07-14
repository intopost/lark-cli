#!/usr/bin/env node

const fs = require("fs");
const os = require("os");
const path = require("path");
const crypto = require("crypto");
const { execFileSync } = require("child_process");

const pkg = require("../package.json");

const VERSION = pkg.version;
const REPO = "intopost/lark-cli";
const NAME = "lark-cli";

const PLATFORM_MAP = {
  darwin: "darwin",
  linux: "linux",
  win32: "windows",
};

const ARCH_MAP = {
  x64: "amd64",
  arm64: "arm64",
};

const platform = PLATFORM_MAP[process.platform];
const arch = ARCH_MAP[process.arch];
const isWindows = process.platform === "win32";
const archiveExt = isWindows ? ".zip" : ".tar.gz";
const binaryName = NAME + (isWindows ? ".exe" : "");
const archiveName = `${NAME}-${VERSION}-${platform}-${arch}${archiveExt}`;
const releaseBaseURL =
  process.env.LARK_CLI_RELEASE_BASE_URL ||
  `https://github.com/${REPO}/releases/download/v${VERSION}`;
const archiveURL = `${releaseBaseURL}/${archiveName}`;
const checksumsURL = `${releaseBaseURL}/checksums.txt`;
const binDir = path.join(__dirname, "..", "bin");
const destBinary = path.join(binDir, binaryName);

function assertSupportedPlatform() {
  if (!platform || !arch) {
    throw new Error(`暂不支持当前平台: ${process.platform}/${process.arch}`);
  }
}

async function fetchBuffer(url) {
  const response = await fetch(url, { redirect: "follow" });
  if (!response.ok) {
    throw new Error(`下载失败: ${url} (${response.status} ${response.statusText})`);
  }
  return Buffer.from(await response.arrayBuffer());
}

async function fetchText(url) {
  const response = await fetch(url, { redirect: "follow" });
  if (!response.ok) {
    throw new Error(`下载失败: ${url} (${response.status} ${response.statusText})`);
  }
  return await response.text();
}

async function downloadFile(url, targetPath) {
  const data = await fetchBuffer(url);
  fs.writeFileSync(targetPath, data);
}

function sha256File(filePath) {
  const hash = crypto.createHash("sha256");
  hash.update(fs.readFileSync(filePath));
  return hash.digest("hex");
}

async function verifyChecksum(archivePath) {
  const content = await fetchText(checksumsURL);
  const expectedLine = content
    .split(/\r?\n/)
    .map((line) => line.trim())
    .find((line) => line.endsWith(`  ${archiveName}`) || line.endsWith(` ${archiveName}`));

  if (!expectedLine) {
    throw new Error(`未在 checksums.txt 中找到 ${archiveName}`);
  }

  const expected = expectedLine.split(/\s+/)[0];
  const actual = sha256File(archivePath);
  if (expected !== actual) {
    throw new Error(`校验失败: expected ${expected}, got ${actual}`);
  }
}

function extractZipWindows(archivePath, destDir) {
  const env = {
    ...process.env,
    LARK_CLI_ARCHIVE: archivePath,
    LARK_CLI_DEST: destDir,
  };
  const command =
    "$ErrorActionPreference='Stop';" +
    "Expand-Archive -LiteralPath $env:LARK_CLI_ARCHIVE -DestinationPath $env:LARK_CLI_DEST -Force";
  execFileSync(
    "powershell.exe",
    ["-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", command],
    { stdio: "inherit", env }
  );
}

function extractArchive(archivePath, destDir) {
  if (isWindows) {
    extractZipWindows(archivePath, destDir);
    return;
  }
  execFileSync("tar", ["-xzf", archivePath, "-C", destDir], { stdio: "inherit" });
}

async function install() {
  assertSupportedPlatform();
  fs.mkdirSync(binDir, { recursive: true });

  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "lark-cli-"));
  const archivePath = path.join(tmpDir, archiveName);
  const extractedBinaryPath = path.join(tmpDir, binaryName);

  try {
    await downloadFile(archiveURL, archivePath);
    await verifyChecksum(archivePath);
    extractArchive(archivePath, tmpDir);

    if (!fs.existsSync(extractedBinaryPath)) {
      throw new Error(`解压后未找到二进制: ${binaryName}`);
    }

    fs.copyFileSync(extractedBinaryPath, destBinary);
    fs.chmodSync(destBinary, 0o755);
    console.log(`${NAME} v${VERSION} 安装成功`);
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

if (require.main === module) {
  install().catch((error) => {
    console.error(error.message || error);
    process.exit(1);
  });
}

module.exports = {
  install,
};
