package ipasstrans

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/larksuite/cli/errs"
	"lark-cli-ipass/envvars"
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
	if post != nil {
		t.Fatalf("post hook = %T, want nil", post)
	}
	if req.Method != http.MethodPost {
		t.Fatalf("method = %s, want POST", req.Method)
	}
	if got := req.URL.String(); got != "http://127.0.0.1:12345/oc_adapter/lark-proxy" {
		t.Fatalf("URL = %q", got)
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
	bodyMap, ok := payload.Body.(map[string]any)
	if !ok || bodyMap["text"] != "hello" {
		t.Fatalf("decoded body = %#v", payload.Body)
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

func TestDecodeBody_MultipartRejected(t *testing.T) {
	_, err := decodeBody([]byte("raw"), "multipart/form-data; boundary=abc")
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
