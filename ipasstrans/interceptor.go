package ipasstrans

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/transport"
	"lark-cli-ipass/envvars"
)

const (
	placeholderUAT = "ipass-managed-uat"
	placeholderTAT = "ipass-managed-tat"

	headerTeamUUID  = "xybot-teamUuid"
	targetTypeIPass = "IPAAS_CONNECTOR"
)

type Provider struct{}

func (p *Provider) Name() string { return "ipass" }

func (p *Provider) ResolveInterceptor(ctx context.Context) transport.Interceptor {
	baseURL := strings.TrimSpace(os.Getenv(envvars.AIPowerBaseURL))
	if baseURL == "" {
		return nil
	}

	cfg := interceptorConfig{
		baseURL:   baseURL,
		apiToken:  strings.TrimSpace(os.Getenv(envvars.AIPowerAPIToken)),
		sessionID: strings.TrimSpace(os.Getenv(envvars.IPassSessionID)),
		runID:     strings.TrimSpace(os.Getenv(envvars.IPassRunID)),
		teamUUID:  strings.TrimSpace(os.Getenv(envvars.IPassTeamUUID)),
	}

	if bindingRaw := strings.TrimSpace(os.Getenv(envvars.AIPowerToolBindingJSON)); bindingRaw != "" {
		if err := json.Unmarshal([]byte(bindingRaw), &cfg.binding); err != nil {
			cfg.configErr = errs.NewValidationError(errs.SubtypeFailedPrecondition,
				"iPass proxy is misconfigured: invalid %s JSON", envvars.AIPowerToolBindingJSON).
				WithHint("inject a valid BizToolBinding JSON string via %s", envvars.AIPowerToolBindingJSON).
				WithCause(err)
		}
	}

	if _, err := parseGatewayBaseURL(cfg.baseURL); err != nil && cfg.configErr == nil {
		cfg.configErr = errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"iPass proxy is misconfigured: invalid %s", envvars.AIPowerBaseURL).
			WithHint("set %s to an absolute http(s) base URL", envvars.AIPowerBaseURL).
			WithCause(err)
	}
	return &Interceptor{cfg: cfg}
}

type interceptorConfig struct {
	baseURL   string
	apiToken  string
	sessionID string
	runID     string
	teamUUID  string
	binding   toolBinding
	configErr error
}

type Interceptor struct {
	cfg interceptorConfig
}

type toolBinding struct {
	UUID          string            `json:"uuid"`
	Name          string            `json:"name"`
	VersionNumber int               `json:"versionNumber"`
	Type          string            `json:"type"`
	BizType       string            `json:"bizType"`
	ToolSetUUID   string            `json:"toolSetUuid"`
	Config        toolBindingConfig `json:"config"`
}

type toolBindingConfig struct {
	ConnectorCode string `json:"connectorCode"`
}

type toolCallRequest struct {
	UUID          string         `json:"uuid"`
	ToolName      string         `json:"toolName"`
	VersionNumber int            `json:"versionNumber"`
	Type          string         `json:"type"`
	TargetID      string         `json:"targetId,omitempty"`
	TargetType    string         `json:"targetType,omitempty"`
	ToolSetUUID   string         `json:"toolSetUuid,omitempty"`
	Params        toolCallParams `json:"params"`
	RunID         string         `json:"runId,omitempty"`
}

type toolCallParams struct {
	TargetMethod string              `json:"target_method"`
	TargetURL    string              `json:"target_url"`
	Identity     string              `json:"identity"`
	Headers      map[string][]string `json:"headers"`
	Body         any                 `json:"body"`
}

type opaqueBody struct {
	Encoding    string `json:"encoding"`
	ContentType string `json:"contentType,omitempty"`
	Data        string `json:"data"`
}

func (i *Interceptor) PreRoundTrip(req *http.Request) func(resp *http.Response, err error) {
	return nil
}

func (i *Interceptor) PreRoundTripE(req *http.Request) (func(resp *http.Response, err error), error) {
	identity := detectIdentity(req)
	if identity == "" {
		return nil, nil
	}
	if i.cfg.configErr != nil {
		return nil, i.cfg.configErr
	}
	if err := validateConfig(i.cfg); err != nil {
		return nil, err
	}

	bodyBytes, err := readAndRestoreBody(req)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeUnknown, "failed to read outgoing request body: %v", err).WithCause(err)
	}

	decodedBody, err := decodeBody(bodyBytes, req.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}

	originalURL := req.URL.String()
	proxyHeaders := cloneHeaders(req.Header)
	delete(proxyHeaders, "Authorization")

	payload := toolCallRequest{
		UUID:          i.cfg.binding.UUID,
		ToolName:      i.cfg.binding.Name,
		VersionNumber: i.cfg.binding.VersionNumber,
		Type:          i.cfg.binding.Type,
		Params: toolCallParams{
			TargetMethod: req.Method,
			TargetURL:    originalURL,
			Identity:     identity,
			Headers:      proxyHeaders,
			Body:         decodedBody,
		},
		RunID: i.cfg.runID,
	}
	if connectorCode := strings.TrimSpace(i.cfg.binding.Config.ConnectorCode); connectorCode != "" {
		payload.TargetID = connectorCode
		payload.TargetType = targetTypeIPass
	}
	if toolSetUUID := strings.TrimSpace(i.cfg.binding.ToolSetUUID); toolSetUUID != "" {
		payload.ToolSetUUID = toolSetUUID
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeSDKError, "failed to marshal iPass proxy request: %v", err).WithCause(err)
	}

	gatewayURL, err := buildGatewayURL(i.cfg.baseURL, i.cfg.sessionID)
	if err != nil {
		return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"iPass proxy is misconfigured: invalid %s", envvars.AIPowerBaseURL).
			WithHint("set %s to an absolute http(s) base URL", envvars.AIPowerBaseURL).
			WithCause(err)
	}

	req.Method = http.MethodPost
	req.URL = gatewayURL
	req.Host = ""
	req.Header = make(http.Header)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+i.cfg.apiToken)
	if i.cfg.teamUUID != "" {
		req.Header.Set(headerTeamUUID, i.cfg.teamUUID)
	}
	req.Body = io.NopCloser(bytes.NewReader(encoded))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(encoded)), nil
	}
	req.ContentLength = int64(len(encoded))

	return nil, nil
}

func validateConfig(cfg interceptorConfig) error {
	if cfg.apiToken == "" {
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"iPass proxy is misconfigured: %s is required", envvars.AIPowerAPIToken).
			WithHint("inject the outer AIPower bearer token via %s", envvars.AIPowerAPIToken)
	}
	if cfg.sessionID == "" {
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"iPass proxy is misconfigured: %s is required", envvars.IPassSessionID).
			WithHint("inject the active session ID into the lark-cli process environment")
	}
	if strings.TrimSpace(cfg.binding.UUID) == "" || strings.TrimSpace(cfg.binding.Name) == "" ||
		strings.TrimSpace(cfg.binding.Type) == "" || cfg.binding.VersionNumber <= 0 {
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"iPass proxy is misconfigured: %s is incomplete", envvars.AIPowerToolBindingJSON).
			WithHint("binding JSON must include uuid, name, type, and versionNumber")
	}
	if strings.TrimSpace(cfg.binding.Config.ConnectorCode) == "" {
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"iPass proxy is misconfigured: %s.config.connectorCode is required", envvars.AIPowerToolBindingJSON).
			WithHint("set connectorCode to the target iPass connector code")
	}
	return nil
}

func detectIdentity(req *http.Request) string {
	if auth := req.Header.Get("Authorization"); auth != "" {
		token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		switch token {
		case placeholderUAT:
			return "user"
		case placeholderTAT:
			return "bot"
		}
	}
	return ""
}

func readAndRestoreBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	bodyBytes, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(bodyBytes)), nil
	}
	return bodyBytes, nil
}

func decodeBody(body []byte, contentType string) (any, error) {
	if len(body) == 0 {
		return nil, nil
	}
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch mediaType {
	case "", "application/json", "text/json":
		var decoded any
		if err := json.Unmarshal(body, &decoded); err == nil {
			return decoded, nil
		}
		if mediaType == "" {
			return string(body), nil
		}
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"failed to decode JSON request body for iPass proxy").WithHint("the outgoing request declared application/json but the body was not valid JSON")
	case "application/x-www-form-urlencoded", "text/plain":
		return string(body), nil
	}
	if strings.HasPrefix(mediaType, "multipart/form-data") {
		return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"iPass proxy does not support multipart business commands yet").
			WithHint("first-stage proxy routing only supports JSON and form-urlencoded payloads")
	}
	if strings.HasPrefix(mediaType, "text/") {
		return string(body), nil
	}
	return opaqueBody{
		Encoding:    "base64",
		ContentType: contentType,
		Data:        base64.StdEncoding.EncodeToString(body),
	}, nil
}

func buildGatewayURL(baseURL, sessionID string) (*url.URL, error) {
	base, err := parseGatewayBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	parts := []string{"/"}
	if trimmed := strings.Trim(base.Path, "/"); trimmed != "" {
		parts = append(parts, trimmed)
	}
	parts = append(parts, "api", "agent", "v2", "oc", "sessions", sessionID, "tools", "call")
	base.Path = path.Join(parts...)
	base.RawQuery = ""
	base.Fragment = ""
	return base, nil
}

func parseGatewayBaseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("scheme must be http or https")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("missing host")
	}
	return u, nil
}

func cloneHeaders(src http.Header) map[string][]string {
	out := make(map[string][]string, len(src))
	for k, vs := range src {
		out[k] = append([]string(nil), vs...)
	}
	return out
}

func init() {
	transport.Register(&Provider{})
}
