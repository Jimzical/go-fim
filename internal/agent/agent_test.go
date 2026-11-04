package agent

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Jimzical/go-fim/internal/client"
	"github.com/Jimzical/go-fim/internal/report"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestSendOrQueue_RateLimitedPreservesReport(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "10")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"detail":"Rate limit exceeded"}`))
	}))
	defer ts.Close()

	c := client.New(ts.URL, "token123", false)
	log := discardLogger()

	dir := t.TempDir()
	w := report.Writer{Dir: dir, MaxN: 5}
	rep := report.Report{AgentID: "11111111-1111-1111-1111-111111111111", AgentName: "agent-test"}
	path, err := w.Save(rep)
	if err != nil {
		t.Fatalf("failed to save report: %v", err)
	}

	// Initial saved report exists
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("saved report does not exist: %v", err)
	}

	sendOrQueue(log, c, rep, path, false)

	// Since 429 was returned, sendOrQueue should have queued for replay (.unsent)
	unsentPath := path + report.UnsentSuffix
	if _, err := os.Stat(unsentPath); err != nil {
		t.Errorf("expected unsent report file %q to exist after 429, got: %v", unsentPath, err)
	}

	pending, err := report.ListUnsent(dir)
	if err != nil {
		t.Fatalf("failed to list unsent: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("expected 1 unsent report file, got %d", len(pending))
	}
}

func TestReplayPending_RateLimitedPreservesPendingReports(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"detail":"Too Many Requests"}`))
	}))
	defer ts.Close()

	c := client.New(ts.URL, "token123", false)
	log := discardLogger()

	dir := t.TempDir()
	w := report.Writer{Dir: dir, MaxN: 5}
	rep := report.Report{AgentID: "11111111-1111-1111-1111-111111111111", AgentName: "agent-test"}
	path, err := w.Save(rep)
	if err != nil {
		t.Fatalf("failed to save report: %v", err)
	}

	// Mark it unsent manually
	if err := report.MarkUnsent(path); err != nil {
		t.Fatalf("failed to mark unsent: %v", err)
	}

	pendingBefore, err := report.ListUnsent(dir)
	if err != nil || len(pendingBefore) != 1 {
		t.Fatalf("expected 1 unsent report before replay, got: %d", len(pendingBefore))
	}

	// Attempt replay when server returns 429
	serverDown := replayPending(log, c, dir)
	if !serverDown {
		t.Errorf("expected replayPending to return true (halt / serverDown) on 429, got false")
	}

	// Verify report file was NOT deleted / dropped
	pendingAfter, err := report.ListUnsent(dir)
	if err != nil {
		t.Fatalf("failed to list unsent after replay: %v", err)
	}
	if len(pendingAfter) != 1 {
		t.Fatalf("expected 1 unsent report to remain on disk after 429, got %d", len(pendingAfter))
	}
	if pendingAfter[0] != pendingBefore[0] {
		t.Errorf("pending file changed: %q vs %q", pendingAfter[0], pendingBefore[0])
	}
}

func TestReplayPending_ProtocolErrorDropsMalformed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest) // 400 Bad Request
		_, _ = w.Write([]byte(`{"detail":"Bad Request"}`))
	}))
	defer ts.Close()

	c := client.New(ts.URL, "token123", false)
	log := discardLogger()

	dir := t.TempDir()
	w := report.Writer{Dir: dir, MaxN: 5}
	rep := report.Report{AgentID: "11111111-1111-1111-1111-111111111111"}
	path, err := w.Save(rep)
	if err != nil {
		t.Fatalf("failed to save report: %v", err)
	}
	if err := report.MarkUnsent(path); err != nil {
		t.Fatalf("failed to mark unsent: %v", err)
	}

	// Attempt replay when server returns permanent 400
	serverDown := replayPending(log, c, dir)
	if serverDown {
		t.Errorf("expected replayPending to return false on permanent protocol error")
	}

	// 400 Bad Request should cause pending file to be dropped
	pendingAfter, err := report.ListUnsent(dir)
	if err != nil {
		t.Fatalf("failed to list unsent: %v", err)
	}
	if len(pendingAfter) != 0 {
		t.Errorf("expected pending report to be dropped on 400, got %d files", len(pendingAfter))
	}
}

func TestComputeFullJitter(t *testing.T) {
	if got := ComputeFullJitter(0); got != 0 {
		t.Errorf("ComputeFullJitter(0) = %v, want 0", got)
	}
	if got := ComputeFullJitter(-5 * time.Second); got != 0 {
		t.Errorf("ComputeFullJitter(-5s) = %v, want 0", got)
	}

	maxJitter := 10 * time.Second
	for i := 0; i < 50; i++ {
		j := ComputeFullJitter(maxJitter)
		if j < 0 || j >= maxJitter {
			t.Fatalf("ComputeFullJitter(%v) returned out of bounds: %v", maxJitter, j)
		}
	}
}
