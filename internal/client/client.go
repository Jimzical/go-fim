// Package client is the agent-side HTTP wrapper around the FastAPI control
// plane. SendReport posts a scan report; RegisterAgent completes the one-shot
// setup handshake (gated by a server-issued JWT). Errors are classified so the
// queue/replay layer knows when to retry vs drop.
package client

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/Jimzical/go-fim/internal/report"
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
	BaseURL  string
	APIToken string // server-issued opaque token; sent as Bearer on every /report
	HTTP     *http.Client
}

func New(baseURL, apiToken string, insecureSkipVerify bool) *Client {
	return &Client{
		BaseURL:  baseURL,
		APIToken: apiToken,
		HTTP: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: insecureSkipVerify},
			},
		},
	}
}

// SendReport POSTs the report. Caller fills rep.AgentID / rep.AgentName /
// rep.ScanPath before calling - the server keys on AgentID and refreshes
// the operator-supplied display fields on every report.
func (c *Client) SendReport(rep report.Report) error {
	return c.post("/report", rep, nil, c.APIToken)
}

// RegisterReq is the body sent to /api/setup. The agent_name and scan_path
// live in the JWT claims, so the server doesn't need them in the body.
type RegisterReq struct {
	AgentID string `json:"agent_id"`
}

// RegisterResp echoes back what the server bound to this agent_id, so the
// CLI can log a confirmation line ("registered prod-web-01 → /var/www").
type RegisterResp struct {
	AgentID   string `json:"agent_id"`
	AgentName string `json:"agent_name"`
	ScanPath  string `json:"scan_path"`
	APIToken  string `json:"api_token"`
}

// RegisterAgent completes the one-shot setup handshake. The token is a JWT
// the operator pasted in from the dashboard; the server validates it and
// inserts the agent row keyed by agentID.
func (c *Client) RegisterAgent(token, agentID string) (*RegisterResp, error) {
	var resp RegisterResp
	if err := c.post("/api/setup", RegisterReq{AgentID: agentID}, &resp, token); err != nil {
		return nil, err
	}
	return &resp, nil
}

// post is the shared encode/dispatch path. out=nil discards the body;
// token="" skips the Authorization header.
func (c *Client) post(path string, body, out any, token string) error {
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
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	defer func() { _ = resp.Body.Close() }()

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
