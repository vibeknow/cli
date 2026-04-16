package httpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// DoUpload sends a multipart/form-data POST with a single file field.
func (c *Client) DoUpload(ctx context.Context, path, fieldName, fileName string, fileBody io.Reader, out any) error {
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)

	go func() {
		part, err := mw.CreateFormFile(fieldName, fileName)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		if _, err := io.Copy(part, fileBody); err != nil {
			pw.CloseWithError(err)
			return
		}
		pw.CloseWithError(mw.Close())
	}()

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, pr)
	if err != nil {
		return fmt.Errorf("httpclient: new request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.http.Do(req)
	if err != nil {
		var eo *errObject
		if errors.As(err, &eo) {
			return eo
		}
		return &errObject{Code: "network_error", Message: err.Error(), Retryable: true}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return parseBackendError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return &errObject{Code: "unknown", Message: "decode response: " + err.Error()}
	}
	return nil
}
