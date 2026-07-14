# iPass 代理：上传 / 下载支持方案

## 背景

`自研Agent接入飞书CLI-业务命令代理方案.md` 第一阶段明确不处理上传下载 / multipart。当前拦截器遇到 `multipart/form-data` 直接报错（`ipasstrans/interceptor.go:189-193`）。

本方案补齐上传下载：真实文件二进制不经过 ipaas，统一走 OSS 中转。

约束不变：本地不落真实凭证；真实请求由 ipaas 代发飞书；尽量不改官方命令代码。

## 核心结论

1. **上传/下载判断统一靠 `Content-Type` / 响应头，不靠 URL 表，也不要求 gateway 理解 CLI 私有 intent**：
   - 上传信号 = CLI 转发过来的请求头 `Content-Type` 是 `multipart/form-data`
   - 下载信号 = 飞书响应是二进制（非 JSON / `application/octet-stream` / 带 `Content-Disposition`）

2. **职责边界要收窄**：
   - CLI 负责识别上传请求、把本地真实文件上传到 OSS，并把转发参数尽量还原成“接近原始请求”的 `method + path + query + headers + body`
   - OC 只负责给 CLI 提供 OSS 上传签名能力（`POST /oc_adapter/oss/upload`）
   - `ipaas_action.js` 只处理最终和飞书的请求，不管理签名，不设计额外协议，只负责：
     - 请求前：把 body 里的文件 URL 下载成二进制，组装真实 multipart
     - 响应后：把飞书返回的二进制上传成 URL，再包成 envelope 回给 CLI

3. **上传仍走 `multipart/form-data` 转 OSS URL 的方案，但 gateway 只做“物化”**：
   - transport 在真正看到 `multipart/form-data` 时，直接解析 multipart body，提取文本字段和文件 part
   - CLI 把文件 part 上传到 OSS，转发到 gateway 时尽量保留原始字段结构，只把文件字节替换成 URL descriptor
   - gateway 不理解上传业务语义，只把 URL descriptor 下载回二进制并组装 multipart

4. **请求头也应尽量保留原始形态**：
   - CLI 转发时应携带原始请求头的可透传子集
   - `ipaas_action.js` 只追加 / 覆盖 `Authorization: Bearer <token>`
   - 若需要重组 multipart，`ipaas_action.js` 不复用原始 `Content-Type`，而是让运行时按新的 `FormData` 自动生成 boundary
   - `Host`、`Content-Length` 等不能原样透传的头由 gateway 丢弃或重算

## 链路设计

### 上传

```
lark-cli buildServiceRequest
  └─ DoAPI
        └─ transport 拦截器 PreRoundTripE   [ipasstrans]
              ├─ 检测 multipart（不再报错）
              ├─ 解析 multipart，提取文本字段和 file part 元数据
              ├─ 调 OC 端点 POST /oc_adapter/oss/upload 申请上传签名
              ├─ PUT 文件字节到 uploadUrl；拿到 readUrl/fileKey
              └─ 组 params{method,path,query,headers,body}
                    │
                    ▼
                  ipaas_action.js：看 headers.Content-Type= multipart/form-data
                    ├─ 下载 body 中的文件 url
                    ├─ 拼真实 multipart
                    └─ 发飞书 → 原样回传
```

要点：
- multipart 里的**文件二进制不经 ipaas 透传**；transport 在确认是上传请求后才把文件转成 OSS 引用。
- CLI 不直接持有 OSS token / AK/SK；上传前统一向 OC 的 `/oc_adapter/oss/upload` 申请临时上传 URL。
- transport 直接使用 multipart 中已经存在的文件字节上传 OSS，不再额外引入本地 fileio 映射层。
- dry-run 安全：只要 dry-run 不真正发请求，就不会触发 OSS 中转。
- gateway 不负责“识别上传业务语义”，只看转发过来的 `headers["Content-Type"]` 是否为 `multipart/form-data`。

### Gateway 职责边界

- `ipaas_action.js` 的输入应尽量接近原始飞书请求：
  - `method`
  - `path`
  - `query`
  - `headers`
  - `body`
- 其中 `body` 应尽量保留原始字段结构；对于 multipart，只替换文件字节，不改普通字段含义。
- 文件字段建议使用轻量 descriptor，例如：

```json
{
  "file": {
    "url": "https://...",
    "filename": "a.png",
    "content_type": "image/png"
  },
  "parent_type": "ccm_import_open",
  "size": "123"
}
```

- `ipaas_action.js` 不负责：
  - 调 OC 申请上传签名
  - 设计新的上传 intent 协议
  - 猜测请求是不是上传（应由 `Content-Type` 决定）
- `ipaas_action.js` 只负责两段转换：
  - 请求前：URL → 二进制 → multipart/form-data
  - 响应后：二进制 → `iPaaSFileUpload(...)` → URL envelope

### OC 侧新增端点

- 路由位置：`/Users/post/code/reworkx/yingdao-ai-opencode/packages/aipower/src/routes/oc.ts`
- 路由形态：由于 `oc.ts` 已 `basePath("/oc_adapter")`，新增端点写成 `POST /oss/upload`，对外即 `POST /oc_adapter/oss/upload`
- 端点职责：
  - 接收 CLI 传入的文件元数据：`fileName`、`contentType`、`contentDisposition?`、`fileType?`、`fileOriginScene`
  - 从当前 OC session / iPass session 解析上传所需鉴权，不把 token 下发给 CLI
  - 内部调用现有上传签名能力（当前可复用 `aipowerClient().upload.temp(...)`）
  - 把 `uploadUrl/readUrl/fileKey/fileUniqueKey/bucketName` 原样返回给 CLI
- 建议请求体：

```json
{
  "fileName": "a.png",
  "contentType": "image/png",
  "contentDisposition": "inline; filename=\"a.png\"",
  "fileType": "image",
  "fileOriginScene": "lark_cli_ipass_upload"
}
```

- 建议响应体：

```json
{
  "code": 200,
  "data": {
    "uploadUrl": "https://...",
    "readUrl": "https://...",
    "fileKey": "xxx",
    "fileUniqueKey": "xxx",
    "bucketName": "xxx"
  }
}
```

### 下载

```
lark-cli DoAPI
  └─ transport 拦截器 PreRoundTripE 返回的 post-hook   [ipasstrans]
        └─（ipaas_action.js：飞书返回二进制 → 调 `iPaaSFileUpload(...)` → 返回 envelope
             {"__ipass_file":{"oss_url","content_type","filename"}}；JSON 则透传）
        ├─ 响应含 __ipass_file → 下载 oss_url 拿二进制
        │     换掉 resp.Body，补回 Content-Type / Content-Disposition
        └─ 无标记 → 原样放过
  └─ HandleResponse / SaveResponse   [官方逻辑，零改动]
        └─ 按现有 --output 流程写本地
```

要点：
- 下载侧 lark-cli 的写入逻辑完全不感知（拿到的就是正常二进制响应）。
- ipaas 不需要「哪些接口是下载」的表，按响应 `Content-Type` / `Content-Disposition` 判断即可。
- 返回给 CLI 的 envelope 仍沿用：

```json
{
  "__ipass_file": {
    "oss_url": "https://...",
    "content_type": "application/pdf",
    "filename": "report.pdf"
  }
}
```

字段名虽然叫 `oss_url`，但这里语义是“CLI 可再次下载的中转 URL”，不要求 gateway 真的理解 OSS 细节。

## 改动清单

| 位置 | 改动 |
|---|---|
| `ipasstrans/interceptor.go:189-193` | multipart 分支：不再报错，改为直接解析 multipart 请求体、调用 `/oc_adapter/oss/upload` 获取签名并上传 OSS；转发参数里补可透传的 `headers`，并尽量保留原始 body 结构 |
| `ipasstrans/interceptor.go:78-80` `PreRoundTripE` | post-hook 从 `return nil,nil` 改为返回处理函数，识别 `__ipass_file` envelope → url 转二进制 |
| `/Users/post/code/reworkx/yingdao-ai-opencode/packages/aipower/src/routes/oc.ts` | 新增 `POST /oss/upload`：代理 AIPower 上传签名能力，向 CLI 返回 `uploadUrl/readUrl/...` |
| params 协议（更新设计文档） | 统一成 `method + path + query + headers + body`；其中 body 尽量保留原始结构，只把文件字节替换成 URL descriptor |
| `ipaas_action.js` | 仅处理最终飞书请求：上传时按 `headers["Content-Type"]` 判断 multipart，下载 URL 组装 `FormData`；下载时按响应头判断二进制，调用 `iPaaSFileUpload(...)` 返回 envelope |

## 待定 / 需确认

- **`/oc_adapter/oss/upload` 的 session 绑定**：用 `x-ipass-session-id`、OC sessionID，还是两者都支持。 用同一个
- **multipart body 的文件 descriptor 具体字段名**：例如固定为 `{url, filename, content_type}`，还是保留更贴近当前 CLI 里已有结构。
- **headers 透传白名单**：除了 `Authorization`/`Host`/`Content-Length` 外，是否还需要明确过滤其他 hop-by-hop 头。

## 验收

- 上传：`drive +upload --file xxx`（或等价命令）经 ipaas 跑通，飞书侧文件正确。
- 下载：下载类命令 `--output xxx` 经 ipaas 跑通，本地文件二进制正确、lark-cli 无改动。
- 本地无真实凭证；二进制不经过 ipaas，只走 OSS。
- 非上传下载命令、`--dry-run`、`schema` 不受影响。
