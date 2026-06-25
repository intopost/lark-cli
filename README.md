# lark-cli

这是一个带 iPass 代理扩展的 `lark-cli` 分发仓库。

## 安装

推荐通过 `npx` 触发全局安装：

```bash
npx @intopost/lark-cli@latest install
```

也可以直接全局安装：

```bash
npm install -g @intopost/lark-cli
```

安装完成后可直接运行：

```bash
lark-cli --help
```

## 工作方式

- npm 包本身只负责分发和安装
- `postinstall` 会按当前平台下载预编译二进制
- `npx @intopost/lark-cli@latest install` 会转成一次全局 `npm install -g`
- 二进制从 GitHub Releases 下载，仓库地址固定为 `https://github.com/intopost/lark-cli`

## 发布

发布流程已经配置在 `.github/workflows/release.yml`：

1. 更新 `package.json` 里的版本号
2. 提交代码并打 tag，例如 `v0.1.0`
3. 推送 tag
4. GitHub Actions 会自动：
   - 运行 `go test ./...`
   - 构建 darwin/linux/windows 的 amd64/arm64 二进制
   - 生成 GitHub Release 和 `checksums.txt`
   - 执行 `npm publish --access public`

## 需要的仓库 Secret

- `NPM_TOKEN`: npm 发布令牌

## 本地调试

```bash
go test ./...
npm pack
node scripts/run.js --help
```
