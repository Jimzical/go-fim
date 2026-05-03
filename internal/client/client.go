// Package client is the agent-side HTTP wrapper around the FastAPI control
// plane. One call (SendReport) — the server lazily creates the agent row on
// first /report. Errors are classified so the queue/replay layer (4e) knows
// when to retry vs drop.
package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"go-fim/internal/report"
)

// ErrUnreachable wraps any failure where retrying later might succeed:
// network/DNS/timeout, plus 5xx from the server. 4xx returns *HTTPError
// instead — those signal a protocol bug, not transient unavailability.
var ErrUnreachable = errors.New("server unreachable")

// HTTPError is a non-2xx, non-5xx response (i.e. 4xx). Don't queue these.
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("server %d: %s", e.StatusCode, e.Body)
}

// Client posts to the go-fim control plane. Single 5s timeout covers
// connect+read; cron-driven agents shouldn't hang the run on a sick server.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 5 * time.Second},
	}
}

// SendReport POSTs the report. Caller fills rep.AgentID / rep.AgentName /
// rep.ScanPath before calling — the server keys on AgentID and refreshes
// the operator-supplied display fields on every report.
func (c *Client) SendReport(rep report.Report) error {
	return c.post("/report", rep, nil)
}

// post is the shared encode/dispatch path. out=nil means "discard body".
func (c *Client) post(path string, body any, out any) error {
	u, err := url.JoinPath(c.BaseURL, path)
	if err != nil {
		return fmt.Errorf("build url: %w", err)
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return fmt.Errorf("encode body: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, u, &buf)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode >= 500:
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: 5xx: %s", ErrUnreachable, string(respBody))
	case resp.StatusCode >= 400:
		respBody, _ := io.ReadAll(resp.Body)
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
