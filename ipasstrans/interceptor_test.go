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
	unsetEnv(t, envvars.AIPowerBaseURL)
	if got := (&Provider{}).ResolveInterceptor(context.Background()); got != nil {
		t.Fatalf("ResolveInterceptor() = %T, want nil", got)
	}
}

func TestInterceptor_PreRoundTripE_RewritesJSONRequest(t *testing.T) {
	interceptor := &Interceptor{cfg: interceptorConfig{
		baseURL:   "https://aipower.example.com/root",
		apiToken:  "api-token",
		sessionID: "sess_123",
		runID:     "run_123",
		teamUUID:  "team_123",
		binding: toolBinding{
			UUID:          "tool_uuid",
			Name:          "lark_cli_proxy",
			VersionNumber: 1,
			Type:          "function",
			ToolSetUUID:   "toolset_uuid",
			Config: toolBindingConfig{
				ConnectorCode: "connector_code",
			},
		},
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
	if got := req.URL.String(); got != "https://aipower.example.com/root/api/agent/v2/oc/sessions/sess_123/tools/call" {
		t.Fatalf("URL = %q", got)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer api-token" {
		t.Fatalf("Authorization = %q", got)
	}
	if got := req.Header.Get(headerTeamUUID); got != "team_123" {
		t.Fatalf("%s = %q", headerTeamUUID, got)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}

	raw, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read rewritten body: %v", err)
	}
	var payload toolCallRequest
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal rewritten body: %v", err)
	}
	if payload.UUID != "tool_uuid" || payload.ToolName != "lark_cli_proxy" || payload.RunID != "run_123" {
		t.Fatalf("unexpected payload envelope: %+v", payload)
	}
	if payload.TargetID != "connector_code" || payload.TargetType != targetTypeIPass {
		t.Fatalf("unexpected target routing: %+v", payload)
	}
	if payload.Params.Identity != "bot" {
		t.Fatalf("identity = %q, want bot", payload.Params.Identity)
	}
	if payload.Params.TargetURL != "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=chat_id" {
		t.Fatalf("target_url = %q", payload.Params.TargetURL)
	}
	if payload.Params.Headers["Authorization"] != nil {
		t.Fatalf("Authorization leaked into proxied headers: %#v", payload.Params.Headers)
	}
	if payload.Params.Headers["X-Cli-Trace"][0] != "trace-1" {
		t.Fatalf("X-Cli-Trace lost: %#v", payload.Params.Headers)
	}
	bodyMap, ok := payload.Params.Body.(map[string]any)
	if !ok || bodyMap["text"] != "hello" {
		t.Fatalf("decoded body = %#v", payload.Params.Body)
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
		baseURL:  "https://aipower.example.com",
		apiToken: "api-token",
		binding: toolBinding{
			UUID:          "tool_uuid",
			Name:          "lark_cli_proxy",
			VersionNumber: 1,
			Type:          "function",
			Config: toolBindingConfig{
				ConnectorCode: "connector_code",
			},
		},
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

func TestInterceptor_PreRoundTripE_InvalidBindingJSON(t *testing.T) {
	setEnv(t, envvars.AIPowerBaseURL, "https://aipower.example.com")
	setEnv(t, envvars.AIPowerToolBindingJSON, "{")

	tr, ok := (&Provider{}).ResolveInterceptor(context.Background()).(*Interceptor)
	if !ok {
		t.Fatalf("ResolveInterceptor() type mismatch")
	}
	req, _ := http.NewRequest(http.MethodGet, "https://open.feishu.cn/open-apis/authen/v1/user_info", nil)
	req.Header.Set("Authorization", "Bearer "+placeholderUAT)

	_, err := tr.PreRoundTripE(req)
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

func TestBuildGatewayURL_NoBasePath(t *testing.T) {
	u, err := buildGatewayURL("https://aipower.example.com", "sess_123")
	if err != nil {
		t.Fatalf("buildGatewayURL() error = %v", err)
	}
	if got := u.String(); got != "https://aipower.example.com/api/agent/v2/oc/sessions/sess_123/tools/call" {
		t.Fatalf("URL = %q", got)
	}
}
