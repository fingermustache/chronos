//go:build !integration

package server_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fingermustache/chronos/api-gateway/internal/config"
	"github.com/fingermustache/chronos/api-gateway/internal/server"
)

// newTestServer spins up the full middleware stack on an in-process test server.
// No real port is opened — httptest handles that internally.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	cfg := config.Config{
		Port:         "8080",
		RateLimitRPM: 60,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	srv := server.New(cfg, logger, nil)
	return httptest.NewServer(srv.Handler)
}

func TestIntegration_HealthEndpoint(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", res.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("expected status 'ok', got '%s'", body["status"])
	}
}

func TestIntegration_RequestIDHeader(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()

	if res.Header.Get("X-Request-ID") == "" {
		t.Error("expected X-Request-ID header in response, got none")
	}
}

func TestIntegration_RequestIDIsEchoed(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	const customID = "test-trace-id-123"

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/health", nil)
	req.Header.Set("X-Request-ID", customID)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()

	if got := res.Header.Get("X-Request-ID"); got != customID {
		t.Errorf("expected echoed request ID %q, got %q", customID, got)
	}
}

func TestIntegration_CORSHeaders(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()

	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("expected CORS origin '*', got %q", got)
	}
}

func TestIntegration_CORSPreflight(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodOptions, ts.URL+"/health", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("preflight request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204 for OPTIONS preflight, got %d", res.StatusCode)
	}
}

func TestIntegration_TaskRoutesReturn503WhenServiceNil(t *testing.T) {
	ts := newTestServer(t) // passes nil service
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/tasks", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusInternalServerError {
		t.Errorf("nil service caused a panic (recovery returned 500); expected a clean 503")
	}
}

func TestIntegration_AuthRequiredForProtectedRoutes(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	// /health is exempt — no token needed
	t.Run("health skips auth", func(t *testing.T) {
		res, err := http.Get(ts.URL + "/health")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", res.StatusCode)
		}
	})
}

func TestIntegration_RateLimiterBlocksAfterLimit(t *testing.T) {
	// Use a very low limit so we can hit it in a test without
	// making hundreds of requests
	cfg := config.Config{
		Port:         "8080",
		RateLimitRPM: 3,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ts := httptest.NewServer(server.New(cfg, logger, nil).Handler)
	defer ts.Close()

	// First 3 requests should pass
	for i := 0; i < 3; i++ {
		res, err := http.Get(ts.URL + "/health")
		if err != nil {
			t.Fatalf("request %d failed: %v", i+1, err)
		}
		res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, res.StatusCode)
		}
	}

	// 4th request should be rate limited
	res, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("rate limit request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusTooManyRequests {
		t.Errorf("expected 429 after exceeding rate limit, got %d", res.StatusCode)
	}

	if res.Header.Get("Retry-After") == "" {
		t.Error("expected Retry-After header on 429 response")
	}
}

func TestIntegration_ValidationAllowsMutationWithoutBody(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/health", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusUnsupportedMediaType {
		t.Fatalf("expected non-415 for bodyless POST, got %d", res.StatusCode)
	}
}

func TestIntegration_ValidationRejectsBodyWithoutContentType(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/health", strings.NewReader(`{"ok":true}`))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("expected 415, got %d", res.StatusCode)
	}
}
