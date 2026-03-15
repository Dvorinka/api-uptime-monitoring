package monitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPCheckUp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	checker := NewChecker()
	result := checker.RunCheck(context.Background(), Monitor{
		Type:           MonitorTypeHTTP,
		Target:         server.URL,
		TimeoutSeconds: 3,
		CheckSSL:       false,
	})

	if result.Status != StatusUp {
		t.Fatalf("expected status up, got %s (err=%q)", result.Status, result.Error)
	}
	if result.HTTPStatus != http.StatusOK {
		t.Fatalf("expected status code %d, got %d", http.StatusOK, result.HTTPStatus)
	}
}

func TestPingCheckInvalidHost(t *testing.T) {
	checker := NewChecker()
	result := checker.RunCheck(context.Background(), Monitor{
		Type:           MonitorTypePing,
		Target:         "invalid host:80",
		TimeoutSeconds: 1,
	})

	if result.Status != StatusDown {
		t.Fatalf("expected status down, got %s", result.Status)
	}
	if result.Error == "" {
		t.Fatalf("expected error message for invalid target")
	}
}
