package ipasstrans

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"

	"github.com/larksuite/cli/errs"
)

const fileOriginScene = "lark_cli_ipass_upload"

type ossUploadRequest struct {
	FileName           string `json:"fileName"`
	ContentType        string `json:"contentType,omitempty"`
	ContentDisposition string `json:"contentDisposition,omitempty"`
	FileType           string `json:"fileType,omitempty"`
	FileOriginScene    string `json:"fileOriginScene"`
}

type ossUploadEnvelope struct {
	Code int            `json:"code"`
	Data *ossUploadData `json:"data"`
	Msg  string         `json:"msg,omitempty"`
}

type ossUploadData struct {
	UploadURL     string `json:"uploadUrl"`
	ReadURL       string `json:"readUrl"`
	FileKey       string `json:"fileKey,omitempty"`
	FileUniqueKey string `json:"fileUniqueKey,omitempty"`
	BucketName    string `json:"bucketName,omitempty"`
}

func (i *Interceptor) buildMultipartProxyBody(ctx context.Context, body []byte, contentType string) (any, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"failed to parse multipart request content-type: %v", err).
			WithCause(err)
	}

	boundary := params["boundary"]
	if boundary == "" {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"multipart request is missing boundary")
	}

	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	fields := make(map[string]any)
	uploadedFiles := 0
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"failed to parse multipart request body: %v", err).
				WithCause(err)
		}

		partName := part.FormName()
		if partName == "" {
			_ = part.Close()
			continue
		}

		partBytes, err := io.ReadAll(part)
		_ = part.Close()
		if err != nil {
			return nil, errs.NewInternalError(errs.SubtypeFileIO,
				"failed to read multipart part %q: %v", partName, err).
				WithCause(err)
		}

		if part.FileName() == "" {
			appendMultipartField(fields, partName, string(partBytes))
			continue
		}

		if uploadedFiles > 0 {
			return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition,
				"iPass proxy currently supports only one file upload per multipart request").
				WithHint("split multi-file uploads into separate requests before routing through iPass")
		}

		fileSpec, err := i.uploadMultipartPart(ctx, part.Header, part.FileName(), partBytes)
		if err != nil {
			return nil, err
		}
		fields[partName] = *fileSpec
		uploadedFiles++
	}

	if uploadedFiles == 0 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"multipart request did not contain any file part")
	}

	payload := make(map[string]any, len(fields))
	for key, value := range fields {
		payload[key] = value
	}
	return payload, nil
}

func appendMultipartField(dst map[string]any, name, value string) {
	if existing, ok := dst[name]; ok {
		switch typed := existing.(type) {
		case []string:
			dst[name] = append(typed, value)
		case string:
			dst[name] = []string{typed, value}
		default:
			dst[name] = value
		}
		return
	}
	dst[name] = value
}

func (i *Interceptor) uploadMultipartPart(ctx context.Context, header textproto.MIMEHeader, filename string, content []byte) (*multipartFile, error) {
	contentType := normalizeUploadContentType(header.Get("Content-Type"), content)
	contentDisposition := strings.TrimSpace(header.Get("Content-Disposition"))
	if contentDisposition == "" {
		contentDisposition = formatHeaderFilename(filename)
	}

	uploadTarget, err := i.requestOSSUpload(ctx, ossUploadRequest{
		FileName:           filename,
		ContentType:        contentType,
		ContentDisposition: contentDisposition,
		FileType:           classifyUploadFileType(contentType),
		FileOriginScene:    fileOriginScene,
	})
	if err != nil {
		return nil, err
	}

	if err := putObjectToOSS(ctx, uploadTarget.UploadURL, contentType, content); err != nil {
		return nil, err
	}

	fileSpec := &multipartFile{
		URL:         uploadTarget.ReadURL,
		FileName:    filename,
		ContentType: contentType,
	}
	return fileSpec, nil
}

func (i *Interceptor) requestOSSUpload(ctx context.Context, payload ossUploadRequest) (*ossUploadData, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeSDKError,
			"failed to marshal OSS upload request: %v", err).
			WithCause(err)
	}

	endpoint := strings.TrimRight(i.cfg.ocAdapterURL, "/") + "/oss/upload"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeSDKError,
			"failed to build OSS upload request: %v", err).
			WithCause(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-ipass-session-id", i.cfg.sessionID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport,
			"failed to request OSS upload session: %v", err).
			WithCause(err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport,
			"failed to read OSS upload session response: %v", err).
			WithCause(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errs.NewNetworkError(errs.SubtypeNetworkServer,
			"OSS upload session request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var envelope ossUploadEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"invalid OSS upload session response: %v", err).
			WithCause(err)
	}
	if envelope.Code != 0 && envelope.Code != 200 {
		message := strings.TrimSpace(envelope.Msg)
		if message == "" {
			message = "unknown error"
		}
		return nil, errs.NewNetworkError(errs.SubtypeNetworkServer,
			"OSS upload session request failed: code=%d msg=%s", envelope.Code, message)
	}
	if envelope.Data == nil || strings.TrimSpace(envelope.Data.UploadURL) == "" || strings.TrimSpace(envelope.Data.ReadURL) == "" {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"OSS upload session response is missing uploadUrl/readUrl")
	}
	return envelope.Data, nil
}

func putObjectToOSS(ctx context.Context, uploadURL, contentType string, content []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(content))
	if err != nil {
		return errs.NewInternalError(errs.SubtypeSDKError,
			"failed to build OSS PUT request: %v", err).
			WithCause(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.ContentLength = int64(len(content))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errs.NewNetworkError(errs.SubtypeNetworkTransport,
			"failed to upload file bytes to OSS: %v", err).
			WithCause(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	raw, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return errs.NewNetworkError(errs.SubtypeNetworkServer,
			"OSS upload failed with status %d", resp.StatusCode)
	}
	return errs.NewNetworkError(errs.SubtypeNetworkServer,
		"OSS upload failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(raw)))
}

func normalizeUploadContentType(contentType string, content []byte) string {
	if mediaType, _, err := mime.ParseMediaType(contentType); err == nil && mediaType != "" {
		return mediaType
	}
	if trimmed := strings.TrimSpace(contentType); trimmed != "" {
		return trimmed
	}
	sniffLen := len(content)
	if sniffLen > 512 {
		sniffLen = 512
	}
	if sniffLen == 0 {
		return "application/octet-stream"
	}
	return http.DetectContentType(content[:sniffLen])
}

func classifyUploadFileType(contentType string) string {
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return "image"
	case strings.HasPrefix(contentType, "video/"):
		return "video"
	case strings.HasPrefix(contentType, "audio/"):
		return "audio"
	default:
		return "file"
	}
}

func formatHeaderFilename(filename string) string {
	if filename == "" {
		return ""
	}
	return fmt.Sprintf("inline; filename=%q", filename)
}
