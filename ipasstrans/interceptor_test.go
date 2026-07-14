package ipasstrans

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"testing"

	"github.com/intopost/lark-cli/envvars"
	"github.com/larksuite/cli/errs"
)

func setEnv(t *testing.T, key, value string) {
	t.Helper()
	old, hadOld := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("set env %s: %v", key, err)
	}
	t.Cleanup(func() {
		if hadOld {
			_ = os.Setenv(key, old)
			return
		}
		_ = os.Unsetenv(key)
	})
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	old, hadOld := os.LookupEnv(key)
	_ = os.Unsetenv(key)
	t.Cleanup(func() {
		if hadOld {
			_ = os.Setenv(key, old)
		}
	})
}

func TestProviderResolveInterceptor_NotActive(t *testing.T) {
	unsetEnv(t, envvars.LarkCLIOCAdapterURL)
	if got := (&Provider{}).ResolveInterceptor(context.Background()); got != nil {
		t.Fatalf("ResolveInterceptor() = %T, want nil", got)
	}
}

func TestInterceptor_PreRoundTripE_RewritesToOCAdapter(t *testing.T) {
	interceptor := &Interceptor{cfg: interceptorConfig{
		ocAdapterURL: "http://127.0.0.1:12345/oc_adapter",
		sessionID:    "sess_123",
	}}

	body := []byte(`{"text":"hello"}`)
	req, err := http.NewRequest(http.MethodPost, "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=chat_id", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+placeholderTAT)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Cli-Trace", "trace-1")

	post, err := interceptor.PreRoundTripE(req)
	if err != nil {
		t.Fatalf("PreRoundTripE() error = %v", err)
	}
	if post == nil {
		t.Fatal("post hook = nil, want non-nil")
	}
	if req.Method != http.MethodPost {
		t.Fatalf("method = %s, want POST", req.Method)
	}
	if got := req.URL.String(); got != "http://127.0.0.1:12345/oc_adapter/ipass-proxy/feishu" {
		t.Fatalf("URL = %q, want %q", got, "http://127.0.0.1:12345/oc_adapter/ipass-proxy/feishu")
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := req.Header.Get("x-ipass-session-id"); got != "sess_123" {
		t.Fatalf("x-ipass-session-id = %q", got)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization should not be set on OC adapter request, got %q", got)
	}

	raw, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read rewritten body: %v", err)
	}
	var payload larkProxyRequest
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal rewritten body: %v", err)
	}
	if payload.Method != "POST" {
		t.Fatalf("method = %q, want POST", payload.Method)
	}
	if payload.Path != "/open-apis/im/v1/messages" {
		t.Fatalf("path = %q, want /open-apis/im/v1/messages", payload.Path)
	}
	if payload.Query["receive_id_type"] != "chat_id" {
		t.Fatalf("query[receive_id_type] = %q, want chat_id", payload.Query["receive_id_type"])
	}
	if payload.Headers["content-type"] != "application/json" {
		t.Fatalf("headers[content-type] = %q, want application/json", payload.Headers["content-type"])
	}
	if payload.Headers["x-cli-trace"] != "trace-1" {
		t.Fatalf("headers[x-cli-trace] = %q, want trace-1", payload.Headers["x-cli-trace"])
	}
	if _, ok := payload.Headers["authorization"]; ok {
		t.Fatalf("authorization should not be forwarded, got %#v", payload.Headers["authorization"])
	}
	bodyMap, ok := payload.Body.(map[string]any)
	if !ok || bodyMap["text"] != "hello" {
		t.Fatalf("decoded body = %#v", payload.Body)
	}
}

func TestInterceptor_PreRoundTripE_MultipartUploadRewritesToOSSReference(t *testing.T) {
	var (
		sessionHeader string
		uploadReqBody ossUploadRequest
		uploadedBytes []byte
	)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oc_adapter/oss/upload":
			sessionHeader = r.Header.Get("x-ipass-session-id")
			defer r.Body.Close()
			if err := json.NewDecoder(r.Body).Decode(&uploadReqBody); err != nil {
				t.Fatalf("decode oss upload request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":200,"data":{"uploadUrl":"` + server.URL + `/put-object","readUrl":"https://oss.example.com/read/object"}}`))
		case "/put-object":
			if r.Method != http.MethodPut {
				t.Fatalf("PUT method = %s", r.Method)
			}
			defer r.Body.Close()
			uploadedBytes, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	interceptor := &Interceptor{cfg: interceptorConfig{
		ocAdapterURL: server.URL + "/oc_adapter",
		sessionID:    "sess_456",
	}}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("parent_type", "ccm_import_open"); err != nil {
		t.Fatalf("WriteField: %v", err)
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="hello.txt"`)
	header.Set("Content-Type", "text/plain")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	if _, err := part.Write([]byte("hello via multipart")); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close multipart writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, "https://open.feishu.cn/open-apis/drive/v1/medias/upload_all", bytes.NewReader(body.Bytes()))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+placeholderTAT)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	post, err := interceptor.PreRoundTripE(req)
	if err != nil {
		t.Fatalf("PreRoundTripE() error = %v", err)
	}
	if post == nil {
		t.Fatal("post hook = nil, want non-nil")
	}
	if sessionHeader != "sess_456" {
		t.Fatalf("x-ipass-session-id = %q, want sess_456", sessionHeader)
	}
	if string(uploadedBytes) != "hello via multipart" {
		t.Fatalf("uploaded bytes = %q", string(uploadedBytes))
	}
	if uploadReqBody.FileName != "hello.txt" {
		t.Fatalf("uploadReqBody.FileName = %q", uploadReqBody.FileName)
	}
	if uploadReqBody.ContentType != "text/plain" {
		t.Fatalf("uploadReqBody.ContentType = %q", uploadReqBody.ContentType)
	}

	raw, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read rewritten body: %v", err)
	}
	var payload larkProxyRequest
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal rewritten body: %v", err)
	}

	bodyMap, ok := payload.Body.(map[string]any)
	if !ok {
		t.Fatalf("decoded body type = %T, want map[string]any", payload.Body)
	}
	if payload.Headers["content-type"] != writer.FormDataContentType() {
		t.Fatalf("headers[content-type] = %q, want %q", payload.Headers["content-type"], writer.FormDataContentType())
	}
	if bodyMap["parent_type"] != "ccm_import_open" {
		t.Fatalf("parent_type = %#v", bodyMap["parent_type"])
	}
	fileMap, ok := bodyMap["file"].(map[string]any)
	if !ok {
		t.Fatalf("file = %#v", bodyMap["file"])
	}
	if fileMap["url"] != "https://oss.example.com/read/object" {
		t.Fatalf("url = %#v", fileMap["url"])
	}
	if fileMap["filename"] != "hello.txt" {
		t.Fatalf("filename = %#v", fileMap["filename"])
	}
	if fileMap["content_type"] != "text/plain" {
		t.Fatalf("content_type = %#v", fileMap["content_type"])
	}
}

func TestInterceptor_PreRoundTripE_PassThroughWithoutPlaceholder(t *testing.T) {
	interceptor := &Interceptor{cfg: interceptorConfig{}}
	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer real-token")

	post, gotErr := interceptor.PreRoundTripE(req)
	if gotErr != nil {
		t.Fatalf("PreRoundTripE() error = %v", gotErr)
	}
	if post != nil {
		t.Fatalf("post hook = %T, want nil", post)
	}
	if got := req.URL.String(); got != "https://example.com" {
		t.Fatalf("URL = %q, want pass-through", got)
	}
}

func TestInterceptor_PreRoundTripE_MissingSessionID(t *testing.T) {
	interceptor := &Interceptor{cfg: interceptorConfig{
		ocAdapterURL: "http://127.0.0.1:12345/oc_adapter",
		configErr: errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"iPass proxy is misconfigured: IPASS_SESSION_ID is required").
			WithHint("inject the active session ID via IPASS_SESSION_ID"),
	}}
	req, _ := http.NewRequest(http.MethodGet, "https://open.feishu.cn/open-apis/authen/v1/user_info", nil)
	req.Header.Set("Authorization", "Bearer "+placeholderUAT)

	_, err := interceptor.PreRoundTripE(req)
	if err == nil {
		t.Fatal("expected error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T: %v", err, err)
	}
	if problem.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("subtype = %q, want %q", problem.Subtype, errs.SubtypeFailedPrecondition)
	}
}

func TestInterceptor_PreRoundTripE_InvalidOCAdapterURL(t *testing.T) {
	setEnv(t, envvars.LarkCLIOCAdapterURL, "://invalid")

	got := (&Provider{}).ResolveInterceptor(context.Background())
	if got == nil {
		t.Fatal("expected interceptor, got nil")
	}
	interceptor, ok := got.(*Interceptor)
	if !ok {
		t.Fatalf("type = %T, want *Interceptor", got)
	}
	if interceptor.cfg.configErr == nil {
		t.Fatal("expected configErr, got nil")
	}
}

func TestInterceptor_PostHook_RewritesDownloadEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/download-object" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("pdf-bytes"))
	}))
	defer server.Close()

	interceptor := &Interceptor{cfg: interceptorConfig{
		ocAdapterURL: "http://127.0.0.1:12345/oc_adapter",
		sessionID:    "sess_789",
	}}

	req, err := http.NewRequest(http.MethodGet, "https://open.feishu.cn/open-apis/drive/v1/files/file_123/download", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+placeholderUAT)

	post, err := interceptor.PreRoundTripE(req)
	if err != nil {
		t.Fatalf("PreRoundTripE() error = %v", err)
	}
	if post == nil {
		t.Fatal("post hook = nil, want non-nil")
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"__ipaas_file":{"oss_url":"` + server.URL + `/download-object","content_type":"application/pdf","filename":"report.pdf"}}`))),
	}

	resp.Header.Set("Content-Type", "application/json")
	post(resp, nil)

	rewritten, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read rewritten response: %v", err)
	}
	if string(rewritten) != "pdf-bytes" {
		t.Fatalf("rewritten body = %q", string(rewritten))
	}
	if got := resp.Header.Get("Content-Type"); got != "application/pdf" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := resp.Header.Get("Content-Disposition"); got != `inline; filename="report.pdf"` {
		t.Fatalf("Content-Disposition = %q", got)
	}
}
