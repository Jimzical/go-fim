package client

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Jimzical/go-fim/internal/report"
)

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected time.Duration
	}{
		{"empty", "", 0},
		{"whitespace", "   ", 0},
		{"invalid string", "invalid-seconds", 0},
		{"negative integer", "-10", 0},
		{"zero integer", "0", 0},
		{"positive integer", "30", 30 * time.Second},
		{"large integer", "120", 120 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseRetryAfter(tt.header)
			if got != tt.expected {
				t.Errorf("ParseRetryAfter(%q) = %v, want %v", tt.header, got, tt.expected)
			}
		})
	}
}

func TestParseRetryAfter_HTTPDate(t *testing.T) {
	future := time.Now().Add(45 * time.Second).UTC()
	dateHeader := future.Format(http.TimeFormat)
	got := ParseRetryAfter(dateHeader)
	if got <= 0 || got > 46*time.Second {
		t.Errorf("ParseRetryAfter(future date %q) = %v, expected ~45s", dateHeader, got)
	}

	past := time.Now().Add(-10 * time.Second).UTC().Format(http.TimeFormat)
	if gotPast := ParseRetryAfter(past); gotPast != 0 {
		t.Errorf("ParseRetryAfter(past date %q) = %v, expected 0", past, gotPast)
	}
}

func TestClient_429Response(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "15")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"detail":"Rate limit exceeded"}`))
	}))
	defer ts.Close()

	c := New(ts.URL, "token123", false)
	rep := report.Report{
		AgentID:   "00000000-0000-0000-0000-000000000001",
		AgentName: "agent-1",
	}

	err := c.SendReport(rep)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected errors.Is(err, ErrRateLimited), got %v", err)
	}

	var rle *RateLimitError
	if !errors.As(err, &rle) {
		t.Fatalf("expected errors.As to succeed for *RateLimitError, got %T", err)
	}

	if rle.StatusCode != 429 {
		t.Errorf("rle.StatusCode = %d, want 429", rle.StatusCode)
	}
	if rle.RetryAfter != 15*time.Second {
		t.Errorf("rle.RetryAfter = %v, want 15s", rle.RetryAfter)
	}

	// Verify it does NOT classify as permanent HTTPError
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		t.Errorf("429 should not be classified as *HTTPError: %v", httpErr)
	}
}

func TestClient_Permanent4xxError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":"Bad request"}`))
	}))
	defer ts.Close()

	c := New(ts.URL, "token123", false)
	rep := report.Report{}
	err := c.SendReport(rep)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if errors.Is(err, ErrRateLimited) {
		t.Error("400 Bad Request should not be ErrRateLimited")
	}

	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *HTTPError for 400 Bad Request, got %T", err)
	}
	if httpErr.StatusCode != 400 {
		t.Errorf("httpErr.StatusCode = %d, want 400", httpErr.StatusCode)
	}
}
