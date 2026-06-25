package ipasstrans

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/transport"
	"lark-cli-ipass/envvars"
)

const (
	placeholderUAT = "ipass-managed-uat"
	placeholderTAT = "ipass-managed-tat"
)

type Provider struct{}

func (p *Provider) Name() string { return "ipass" }

func (p *Provider) ResolveInterceptor(ctx context.Context) transport.Interceptor {
	ocAdapterURL := strings.TrimSpace(os.Getenv(envvars.LarkCLIOCAdapterURL))
	if ocAdapterURL == "" {
		return nil
	}

	cfg := interceptorConfig{
		ocAdapterURL: ocAdapterURL,
		sessionID:    strings.TrimSpace(os.Getenv(envvars.IPassSessionID)),
	}

	if cfg.sessionID == "" {
		cfg.configErr = errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"iPass proxy is misconfigured: %s is required", envvars.IPassSessionID).
			WithHint("inject the active session ID via %s", envvars.IPassSessionID)
	}

	if _, err := url.ParseRequestURI(cfg.ocAdapterURL); err != nil && cfg.configErr == nil {
		cfg.configErr = errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"iPass proxy is misconfigured: invalid %s", envvars.LarkCLIOCAdapterURL).
			WithHint("set %s to the OC adapter base URL (e.g. http://127.0.0.1:PORT/oc_adapter)", envvars.LarkCLIOCAdapterURL).
			WithCause(err)
	}

	return &Interceptor{cfg: cfg}
}

type interceptorConfig struct {
	ocAdapterURL string
	sessionID    string
	configErr    error
}

type Interceptor struct {
	cfg interceptorConfig
}

type larkProxyRequest struct {
	Method string            `json:"method"`
	Path   string            `json:"path"`
	Query  map[string]string `json:"query"`
	Body   any               `json:"body"`
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

	bodyBytes, err := readAndRestoreBody(req)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeUnknown, "failed to read outgoing request body: %v", err).WithCause(err)
	}

	decodedBody, err := decodeBody(bodyBytes, req.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	queryMap := make(map[string]string, len(q))
	for k, v := range q {
		queryMap[k] = v[0]
	}
	payload := larkProxyRequest{
		Method: req.Method,
		Path:   req.URL.Path,
		Query:  queryMap,
		Body:   decodedBody,
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeSDKError, "failed to marshal lark proxy request: %v", err).WithCause(err)
	}

	targetURL := strings.TrimRight(i.cfg.ocAdapterURL, "/") + "/ipass-proxy/feishu"
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"iPass proxy is misconfigured: invalid %s", envvars.LarkCLIOCAdapterURL).
			WithCause(err)
	}

	req.Method = http.MethodPost
	req.URL = parsed
	req.Host = ""
	req.Header = make(http.Header)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-ipass-session-id", i.cfg.sessionID)
	req.Body = io.NopCloser(bytes.NewReader(encoded))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(encoded)), nil
	}
	req.ContentLength = int64(len(encoded))

	return nil, nil
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

func init() {
	transport.Register(&Provider{})
}
