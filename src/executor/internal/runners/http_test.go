package runners_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fingermustache/chronos/executor/internal/runners"
)

func TestHTTPRunner_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	runner := runners.NewHTTPRunner()
	result, err := runner.Run(context.Background(), map[string]any{"url": srv.URL, "method": "GET"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", result.StatusCode)
	}
	if result.Output != "ok" {
		t.Errorf("expected output %q, got %q", "ok", result.Output)
	}
}

func TestHTTPRunner_DefaultsToGET(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	runner := runners.NewHTTPRunner()
	_, err := runner.Run(context.Background(), map[string]any{"url": srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("expected method GET, got %s", gotMethod)
	}
}

func TestHTTPRunner_SetsMethod(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	runner := runners.NewHTTPRunner()
	_, err := runner.Run(context.Background(), map[string]any{"url": srv.URL, "method": "POST"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("expected method POST, got %s", gotMethod)
	}
}

func TestHTTPRunner_SetsHeaders(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Custom")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	runner := runners.NewHTTPRunner()
	_, err := runner.Run(context.Background(), map[string]any{
		"url":     srv.URL,
		"headers": map[string]any{"X-Custom": "test-value"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotHeader != "test-value" {
		t.Errorf("expected header value %q, got %q", "test-value", gotHeader)
	}
}

func TestHTTPRunner_SendsBody(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	runner := runners.NewHTTPRunner()
	_, err := runner.Run(context.Background(), map[string]any{
		"url":    srv.URL,
		"method": "POST",
		"body":   `{"hello":"world"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(gotBody) != `{"hello":"world"}` {
		t.Errorf("expected body %q, got %q", `{"hello":"world"}`, string(gotBody))
	}
}

func TestHTTPRunner_NonSuccessIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer srv.Close()

	runner := runners.NewHTTPRunner()
	result, err := runner.Run(context.Background(), map[string]any{"url": srv.URL})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
	if result.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status 500 in result, got %d", result.StatusCode)
	}
	if result.Output != "server error" {
		t.Errorf("expected output %q, got %q", "server error", result.Output)
	}
}

func TestHTTPRunner_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	runner := runners.NewHTTPRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := runner.Run(ctx, map[string]any{"url": srv.URL})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestHTTPRunner_ConnectionRefused(t *testing.T) {
	runner := runners.NewHTTPRunner()
	_, err := runner.Run(context.Background(), map[string]any{"url": "http://127.0.0.1:1"})
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}
}

func TestHTTPRunner_MissingURL(t *testing.T) {
	runner := runners.NewHTTPRunner()
	_, err := runner.Run(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing url, got nil")
	}
}
