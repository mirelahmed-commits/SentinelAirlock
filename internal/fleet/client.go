package fleet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Client is the Sentinel-side fleet client. Every call uses a short, bounded
// HTTP timeout (ClientTimeout) so a slow or unreachable control plane can
// never block for long -- and the caller (internal/cli/sentinel.go) runs
// every Client call from a goroutine entirely independent of the recorder,
// so even a full timeout never delays local filesystem enforcement. This is
// the mechanical enforcement of the fleet-foundation's non-negotiable
// disconnected-operation requirement.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewClient builds a Client for baseURL (the control plane's address).
// token is optional; when set it is sent as both a Bearer Authorization
// header and X-Airlock-Fleet-Token, matching Server.authorized.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   strings.TrimSpace(token),
		http:    &http.Client{Timeout: ClientTimeout},
	}
}

// Enroll registers or refreshes this Sentinel's identity with the control
// plane.
func (c *Client) Enroll(req EnrollRequest) error {
	var resp EnrollResponse
	return c.post("/api/fleet/enroll", req, &resp)
}

// Heartbeat reports current liveness/status/counters. It is safe to call
// even if Enroll has never succeeded -- the control plane creates a minimal
// record from whatever a heartbeat carries rather than rejecting it.
func (c *Client) Heartbeat(req HeartbeatRequest) error {
	var resp HeartbeatResponse
	return c.post("/api/fleet/heartbeat", req, &resp)
}

func (c *Client) post(path string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fleet %s: %s", path, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
