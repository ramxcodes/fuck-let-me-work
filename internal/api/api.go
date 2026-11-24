// Package api is the contract between the daemon and its callers (CLI, tray, page).
// Every endpoint returns the full focus.Status, or {"error": "..."} with a 4xx/5xx code.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/ramxcodes/fuck-let-me-work/internal/focus"
)

// Header must be on every API request. Browsers cannot send a custom header to
// another origin without a CORS preflight, which the daemon never approves,
// so other websites cannot call the API.
const Header = "X-FLMW"

type FocusRequest struct {
	Duration focus.Duration `json:"duration,omitempty"` // 0 = until stopped
}

type NameRequest struct {
	Name string `json:"name"`
}

type ScheduleRequest struct {
	Cron     string         `json:"cron"`
	Duration focus.Duration `json:"duration"`
}

type Error struct {
	Error string `json:"error"`
}

type Client struct {
	Base string
	HTTP *http.Client
}

// DefaultURL is where the daemon listens. FLMW_URL overrides it (used by tests).
func DefaultURL() string {
	if u := os.Getenv("FLMW_URL"); u != "" {
		return u
	}
	return "http://127.0.0.1"
}

func NewClient() *Client {
	return &Client{Base: DefaultURL(), HTTP: &http.Client{Timeout: 5 * time.Second}}
}

func (c *Client) Status(ctx context.Context) (focus.Status, error) {
	return c.do(ctx, http.MethodGet, "/api/status", nil)
}

func (c *Client) Start(ctx context.Context, d time.Duration) (focus.Status, error) {
	return c.do(ctx, http.MethodPost, "/api/focus", FocusRequest{Duration: focus.Duration(d)})
}

func (c *Client) Stop(ctx context.Context) (focus.Status, error) {
	return c.do(ctx, http.MethodDelete, "/api/focus", nil)
}

// Kind is "sites" or "apps".
type Kind string

const (
	Sites Kind = "sites"
	Apps  Kind = "apps"
)

func (c *Client) Add(ctx context.Context, k Kind, name string) (focus.Status, error) {
	return c.do(ctx, http.MethodPost, "/api/"+string(k), NameRequest{Name: name})
}

func (c *Client) Remove(ctx context.Context, k Kind, name string) (focus.Status, error) {
	return c.do(ctx, http.MethodDelete, "/api/"+string(k)+"/"+url.PathEscape(name), nil)
}

func (c *Client) AddSchedule(ctx context.Context, cron string, d time.Duration) (focus.Status, error) {
	return c.do(ctx, http.MethodPost, "/api/schedules", ScheduleRequest{Cron: cron, Duration: focus.Duration(d)})
}

func (c *Client) RemoveSchedule(ctx context.Context, id string) (focus.Status, error) {
	return c.do(ctx, http.MethodDelete, "/api/schedules/"+url.PathEscape(id), nil)
}

func (c *Client) do(ctx context.Context, method, path string, body any) (focus.Status, error) {
	var st focus.Status
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return st, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, &buf)
	if err != nil {
		return st, err
	}
	req.Header.Set(Header, "1")
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return st, fmt.Errorf("daemon not reachable at %s (is it installed and running? try: sudo flmw install): %w", c.Base, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		var e Error
		if json.NewDecoder(res.Body).Decode(&e) != nil || e.Error == "" {
			e.Error = res.Status
		}
		return st, fmt.Errorf("daemon: %s", e.Error)
	}
	return st, json.NewDecoder(res.Body).Decode(&st)
}
