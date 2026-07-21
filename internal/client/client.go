// SPDX-License-Identifier: MPL-2.0

// Package client is a thin wrapper over the HackerOne v1 REST API
// (https://api.hackerone.com/customer-resources/). It speaks JSON:API and
// authenticates with HTTP Basic auth using an API username + token.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the HackerOne API v1 root.
	DefaultBaseURL = "https://api.hackerone.com/v1"

	// defaultMaxRetries bounds retries on 429/5xx responses.
	defaultMaxRetries = 4
)

// Client talks to the HackerOne API.
type Client struct {
	httpClient *http.Client
	baseURL    string
	username   string
	token      string
	userAgent  string
	maxRetries int
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the API root (useful for tests).
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = u } }

// WithHTTPClient injects a custom *http.Client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.httpClient = h } }

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) Option { return func(c *Client) { c.userAgent = ua } }

// New builds a Client. username/token are the API identity credentials used as
// HTTP Basic auth (see the HackerOne "API Token" settings page).
func New(username, token string, opts ...Option) *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    DefaultBaseURL,
		username:   username,
		token:      token,
		userAgent:  "terraform-provider-hackerone",
		maxRetries: defaultMaxRetries,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// APIError is a structured error decoded from a JSON:API error response.
type APIError struct {
	StatusCode int
	Errors     []struct {
		Status string `json:"status"`
		Title  string `json:"title"`
		Detail string `json:"detail"`
	} `json:"errors"`
	raw string
}

func (e *APIError) Error() string {
	if len(e.Errors) > 0 {
		return fmt.Sprintf("hackerone api error (status %d): %s: %s",
			e.StatusCode, e.Errors[0].Title, e.Errors[0].Detail)
	}
	return fmt.Sprintf("hackerone api error (status %d): %s", e.StatusCode, e.raw)
}

// NotFound reports whether an error is a 404 from the API. Resources use this
// to translate a missing remote object into a state removal.
func NotFound(err error) bool {
	if ae, ok := err.(*APIError); ok {
		return ae.StatusCode == http.StatusNotFound
	}
	return false
}

// do executes a request against path (relative to baseURL), marshalling body
// (if non-nil) as JSON and decoding a successful response into out (if non-nil).
// It retries on 429 and 5xx with a bounded backoff that honors Retry-After.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		payload = b
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}

		var reader io.Reader
		if payload != nil {
			reader = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.SetBasicAuth(c.username, c.token)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.userAgent)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("http request: %w", err)
			continue // transient transport error; retry
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		// Retry on rate limit / server errors.
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = decodeAPIError(resp.StatusCode, respBody)
			if wait := retryAfter(resp); wait > 0 && attempt < c.maxRetries {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(wait):
				}
			}
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return decodeAPIError(resp.StatusCode, respBody)
		}

		if out != nil && len(respBody) > 0 {
			if err := json.Unmarshal(respBody, out); err != nil {
				return fmt.Errorf("decode response: %w", err)
			}
		}
		return nil
	}
	return lastErr
}

func decodeAPIError(status int, body []byte) error {
	ae := &APIError{StatusCode: status, raw: string(body)}
	_ = json.Unmarshal(body, ae) // best effort; raw retained on failure
	return ae
}

// retryAfter parses a Retry-After header (seconds form) if present.
func retryAfter(resp *http.Response) time.Duration {
	if v := resp.Header.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			return time.Duration(secs) * time.Second
		}
	}
	return 0
}

// relativeToBase converts an absolute API URL (e.g. from a links.next field)
// into the path+query relative to baseURL, so do() can rebuild the request.
// Returns false if absURL is not under baseURL.
func relativeToBase(absURL, baseURL string) (string, bool) {
	if after, ok := strings.CutPrefix(absURL, baseURL); ok {
		return after, true
	}
	return "", false
}

// backoff returns an exponential delay for the given (1-based) attempt.
func backoff(attempt int) time.Duration {
	d := time.Duration(1<<uint(attempt-1)) * time.Second
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}
