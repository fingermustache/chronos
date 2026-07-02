package runners

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const maxOutputBytes = 64 * 1024 // 64KB

type httpRunner struct {
	client *http.Client
}

func NewHTTPRunner() TaskRunner {
	return &httpRunner{client: &http.Client{}}
}

func (r *httpRunner) Run(ctx context.Context, config map[string]any) (Result, error) {
	url, _ := config["url"].(string)
	if url == "" {
		return Result{}, fmt.Errorf("http runner: missing required field 'url'")
	}

	method := http.MethodGet
	if m, ok := config["method"].(string); ok && m != "" {
		method = m
	}

	var bodyReader io.Reader
	if body, ok := config["body"].(string); ok && body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return Result{}, fmt.Errorf("http runner: build request: %w", err)
	}

	if headers, ok := config["headers"].(map[string]any); ok {
		for k, v := range headers {
			if vs, ok := v.(string); ok {
				req.Header.Set(k, vs)
			}
		}
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("http runner: execute request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxOutputBytes))
	if err != nil {
		return Result{}, fmt.Errorf("http runner: read response body: %w", err)
	}

	result := Result{
		StatusCode: resp.StatusCode,
		Output:     string(raw),
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, fmt.Errorf("http runner: non-2xx status %d", resp.StatusCode)
	}

	return result, nil
}
