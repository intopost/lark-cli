package ipasstrans

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func (i *Interceptor) rewriteDownloadResponse(ctx context.Context, resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}

	raw, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		writeProxyDownloadFailure(resp, http.StatusBadGateway, "failed to read proxy response body")
		return
	}

	envelope, ok := decodeFileEnvelope(raw, resp.Header.Get("Content-Type"))
	if !ok || envelope.IPassFile == nil || strings.TrimSpace(envelope.IPassFile.OSSURL) == "" {
		resp.Body = io.NopCloser(bytes.NewReader(raw))
		resp.ContentLength = int64(len(raw))
		return
	}

	fileBytes, err := downloadProxyFile(ctx, envelope.IPassFile.OSSURL)
	if err != nil {
		writeProxyDownloadFailure(resp, http.StatusBadGateway, err.Error())
		return
	}

	resp.Body = io.NopCloser(bytes.NewReader(fileBytes))
	resp.ContentLength = int64(len(fileBytes))
	if contentType := strings.TrimSpace(envelope.IPassFile.ContentType); contentType != "" {
		resp.Header.Set("Content-Type", contentType)
	}
	if filename := strings.TrimSpace(envelope.IPassFile.FileName); filename != "" {
		resp.Header.Set("Content-Disposition", formatHeaderFilename(filename))
	}
}

func decodeFileEnvelope(body []byte, contentType string) (*ipassFileEnvelope, bool) {
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if mediaType != "" && mediaType != "application/json" && mediaType != "text/json" {
		return nil, false
	}

	var envelope ipassFileEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, false
	}
	return &envelope, envelope.IPassFile != nil
}

func downloadProxyFile(ctx context.Context, downloadURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &proxyDownloadError{
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(body)),
		}
	}
	return body, nil
}

func writeProxyDownloadFailure(resp *http.Response, statusCode int, message string) {
	body := []byte(`{"code":502,"msg":"` + escapeJSON(message) + `"}`)
	resp.StatusCode = statusCode
	resp.Status = http.StatusText(statusCode)
	resp.Header.Set("Content-Type", "application/json")
	resp.Header.Del("Content-Disposition")
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
}

func escapeJSON(s string) string {
	encoded, err := json.Marshal(s)
	if err != nil {
		return "download proxy failure"
	}
	return strings.Trim(string(encoded), `"`)
}

type proxyDownloadError struct {
	StatusCode int
	Body       string
}

func (e *proxyDownloadError) Error() string {
	if e == nil {
		return "download proxy failure"
	}
	if e.Body == "" {
		return "failed to download file from OSS"
	}
	return "failed to download file from OSS: status=" + http.StatusText(e.StatusCode) + " body=" + e.Body
}
