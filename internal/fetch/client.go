// Package fetch downloads a single folder from a GitHub repository over HTTP,
// without requiring git. It uses the Trees API plus raw content for minimal
// downloads and falls back to the repo tarball for large folders or when the
// anonymous rate limit is hit.
package fetch

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	apiBase = "https://api.github.com"
	rawBase = "https://raw.githubusercontent.com"

	// tarballThreshold is the file count above which we skip per-file fetches
	// and grab the whole repo tarball in a single request instead.
	tarballThreshold = 40
)

// Client performs GitHub HTTP requests. The zero value is not usable; use New.
type Client struct {
	http  *http.Client
	token string
}

// New returns a Client. token may be empty for anonymous access to public repos.
func New(token string) *Client {
	return &Client{
		http:  &http.Client{Timeout: 60 * time.Second},
		token: token,
	}
}

// rateLimitError signals that GitHub refused the request due to rate limiting,
// which the orchestrator uses to trigger the tarball fallback.
type rateLimitError struct{ status int }

func (e rateLimitError) Error() string {
	return fmt.Sprintf("github rate limit hit (HTTP %d)", e.status)
}

func isRateLimit(err error) bool {
	_, ok := err.(rateLimitError)
	return ok
}

// do issues a GET request, applying auth and standard headers. The caller owns
// closing the returned body. accept overrides the Accept header when non-empty.
func (c *Client) do(rawurl, accept string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, rawurl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "gh-get")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return c.http.Do(req)
}

// getJSON fetches rawurl and returns the body bytes, mapping 403/429 to a
// rateLimitError and other non-2xx codes to a generic error.
func (c *Client) get(rawurl, accept string) ([]byte, error) {
	resp, err := c.do(rawurl, accept)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		return nil, rateLimitError{status: resp.StatusCode}
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("not found (HTTP 404): %s", rawurl)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, fmt.Errorf("unexpected status HTTP %d for %s", resp.StatusCode, rawurl)
	}
	return io.ReadAll(resp.Body)
}
