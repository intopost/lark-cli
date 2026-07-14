/**
 * 专家模式代码
 * @param {Context} input - 上下文参数
 * @returns {Promise<Record<string, unknown>>}
 */
async function main(input) {
  const { context } = input;
  const { params, property } = context;
  const { auth } = property;
  const { token } = auth.outputs || {};
  if (!token) throw new Error("缺少 auth.outputs.token");

  const method = params["method"];
  const rawPath = params["path"];
  const query = params["query"];
  const body = params["body"];
  const inputHeaders = normalizeHeaders(params["headers"]);

  if (!method) throw new Error("缺少请求方法 method");
  if (!rawPath) throw new Error("缺少请求路径 path");

  console.log(`[Lark API Request] Starting...`);
  console.log(`[Lark API Request] Method: ${method}, RawPath: ${rawPath}`);
  console.log(`[Lark API Request] Input Query:`, query);
  console.log(`[Lark API Request] Input Headers:`, redactHeaders(inputHeaders));
  console.log(`[Lark API Request] Input Body:`, body);

  let url = rawPath.trim();
  if (!url.startsWith("http://") && !url.startsWith("https://")) {
    if (!url.startsWith("/")) {
      url = "/" + url;
    }
    url = `https://open.feishu.cn${url}`;
  }

  if (query) {
    try {
      const queryObj = typeof query === "string" ? JSON.parse(query) : query;
      const urlObj = new URL(url);
      for (const key of Object.keys(queryObj)) {
        if (queryObj[key] !== undefined && queryObj[key] !== null) {
          urlObj.searchParams.append(key, String(queryObj[key]));
        }
      }
      url = urlObj.toString();
    } catch (e) {
      console.error(`[Lark API Request] Query parsing failed:`, e.message);
      throw new Error("Query 参数解析失败，请确保传入了合法的 JSON 字符串: " + e.message);
    }
  }

  console.log(`[Lark API Request] Final URL: ${url}`);

  const reqHeaders = buildRequestHeaders(inputHeaders, token);
  const isMultipart = isMultipartRequest(inputHeaders);

  let reqBody = undefined;
  if (isMultipart) {
    reqBody = await buildMultipartBody(body);
    delete reqHeaders["content-type"];
  } else if (body) {
    if (typeof body === "string") {
      reqBody = body;
    } else {
      reqBody = JSON.stringify(body);
      if (!reqHeaders["content-type"] && (method === "POST" || method === "PUT" || method === "PATCH")) {
        reqHeaders["content-type"] = "application/json; charset=utf-8";
      }
    }
  }

  console.log(`[Lark API Request] Request Headers:`, redactHeaders(reqHeaders));
  console.log(`[Lark API Request] Request Body:`, describeBody(reqBody));

  const response = await fetch(url, {
    method,
    headers: reqHeaders,
    body: reqBody,
  });

  console.log(`[Lark API Request] Response Status: ${response.status} ${response.statusText}`);

  if (isBinaryResponse(response, inputHeaders)) {
    if (!response.ok) {
      const errorText = await response.text().catch(() => "");
      throw new Error(errorText || `请求失败: ${response.status} ${response.statusText}`);
    }
    const envelope = await buildBinaryEnvelope(response);
    console.log(`[Lark API Request] Binary Response Envelope:`, envelope);
    return envelope;
  }

  const text = await response.text();
  console.log(`[Lark API Request] Response Text:`, text);
  let json;
  try {
    json = JSON.parse(text);
  } catch (e) {
    throw new Error(`响应解析失败: ${text}`);
  }

  return json;
}

function normalizeHeaders(input) {
  const parsed = parseObjectInput(input, "headers", {});

  const headers = {};
  for (const key of Object.keys(parsed)) {
    if (parsed[key] === undefined || parsed[key] === null) continue;
    const normalizedKey = String(key).toLowerCase();
    if (normalizedKey === "authorization") continue;
    headers[normalizedKey] = String(parsed[key]);
  }
  return headers;
}

function parseObjectInput(input, fieldName, emptyValue) {
  if (input === undefined || input === null || input === "") return emptyValue;

  let parsed = input;
  if (typeof input === "string") {
    try {
      parsed = JSON.parse(input);
    } catch (e) {
      throw new Error(`${fieldName} 参数解析失败，请确保传入了合法的 JSON 字符串: ${e.message}`);
    }
  }

  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new Error(`${fieldName} 参数必须是 JSON 对象`);
  }
  return parsed;
}

function buildRequestHeaders(inputHeaders, token) {
  const reqHeaders = { ...inputHeaders };
  delete reqHeaders["authorization"];
  delete reqHeaders["host"];
  delete reqHeaders["content-length"];
  delete reqHeaders["connection"];
  delete reqHeaders["transfer-encoding"];
  reqHeaders["authorization"] = `Bearer ${token}`;
  return reqHeaders;
}

function isMultipartRequest(headers) {
  const contentType = headers["content-type"] || "";
  return contentType.toLowerCase().startsWith("multipart/form-data");
}

async function buildMultipartBody(body) {
  const parsedBody = parseObjectInput(body, "multipart 请求 body", {});

  const form = new FormData();
  for (const [key, value] of Object.entries(parsedBody)) {
    await appendMultipartValue(form, key, value);
  }
  return form;
}

async function appendMultipartValue(form, key, value) {
  if (value === undefined || value === null) return;

  if (Array.isArray(value)) {
    for (const item of value) {
      await appendMultipartValue(form, key, item);
    }
    return;
  }

  if (isFileDescriptor(value)) {
    const file = await downloadFileFromURL(value);
    form.append(key, file.buffer, {
      filename: value.filename,
      contentType: file.contentType,
      knownLength: file.buffer.length,
    });
    return;
  }

  if (typeof value === "object") {
    form.append(key, JSON.stringify(value));
    return;
  }

  form.append(key, String(value));
}

function isFileDescriptor(value) {
  return !!value &&
    typeof value === "object" &&
    !Array.isArray(value) &&
    typeof value.url === "string" &&
    typeof value.filename === "string";
}

async function downloadFileFromURL(desc) {
  const response = await fetch(desc.url);
  if (!response.ok) {
    const text = await response.text().catch(() => "");
    throw new Error(`下载上传文件失败: ${response.status} ${text}`);
  }
  const bytes = await response.arrayBuffer();
  return {
    buffer: Buffer.from(bytes),
    contentType: typeof desc.content_type === "string" && desc.content_type
      ? desc.content_type
      : "application/octet-stream",
  };
}

function isBinaryResponse(response, requestHeaders) {
  const shortcut = (requestHeaders["x-cli-shortcut"] || "").trim().toLowerCase();
  if (shortcut === "drive:+download") return true;

  const contentType = (response.headers.get("content-type") || "").toLowerCase();
  const disposition = (response.headers.get("content-disposition") || "").toLowerCase();

  if (disposition) return true;
  if (!contentType) return false;
  if (contentType.includes("application/json") || contentType.includes("text/json")) return false;
  if (contentType.startsWith("text/")) return false;
  return true;
}

async function buildBinaryEnvelope(response) {
  const contentType = response.headers.get("content-type") || "application/octet-stream";
  const filename = parseFilename(response.headers.get("content-disposition")) || guessFilename(response.url, contentType);
  const bytes = await response.arrayBuffer();
  const file = new File([bytes], filename, { type: contentType });
  const storagePath = `/ipaas/lark-cli/${Date.now()}-${filename}`;
  const url = await iPaaSFileUpload(file, storagePath, filename);

  return {
    __ipaas_file: {
      oss_url: url,
      content_type: contentType,
      filename,
    },
  };
}

function parseFilename(disposition) {
  if (!disposition) return "";

  const utf8 = /filename\*=UTF-8''([^;]+)/i.exec(disposition);
  if (utf8 && utf8[1]) {
    try {
      return decodeURIComponent(utf8[1]);
    } catch {
      return utf8[1];
    }
  }

  const plain = /filename="([^"]+)"/i.exec(disposition) || /filename=([^;]+)/i.exec(disposition);
  return plain && plain[1] ? plain[1].trim() : "";
}

function guessFilename(url, contentType) {
  try {
    const parsed = new URL(url);
    const name = parsed.pathname.split("/").filter(Boolean).pop();
    if (name) return decodeURIComponent(name);
  } catch { }

  if (contentType.includes("pdf")) return "download.pdf";
  if (contentType.includes("png")) return "download.png";
  if (contentType.includes("jpeg") || contentType.includes("jpg")) return "download.jpg";
  if (contentType.includes("gif")) return "download.gif";
  if (contentType.includes("webp")) return "download.webp";
  if (contentType.includes("zip")) return "download.zip";
  return "download.bin";
}

function redactHeaders(headers) {
  return Object.keys(headers).reduce((acc, key) => {
    acc[key] = key === "authorization" ? "Bearer ***" : headers[key];
    return acc;
  }, {});
}

function describeBody(body) {
  if (body === undefined) return undefined;
  if (typeof body === "string") return body;
  if (body instanceof FormData) return "[FormData]";
  return body;
}
