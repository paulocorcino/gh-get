// Package fetch downloads a single folder from a GitHub repository over HTTP,
// without requiring git. It uses the Trees API plus raw content for minimal
// downloads and falls back to the repo tarball for large folders or when the
// anonymous rate limit is hit.
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Endpoints; variables only so tests can point them at a local server.
var (
	apiBase      = "https://api.github.com"
	rawBase      = "https://raw.githubusercontent.com"
	codeloadBase = "https://codeload.github.com"
	gitBase      = "https://github.com"
)

const (
	// tarballThreshold is the file count above which we skip per-file fetches
	// and grab the whole repo tarball in a single request instead.
	tarballThreshold = 40

	// headerTimeout bounds the wait for a response's headers.
	headerTimeout = 30 * time.Second
	// stallTimeout bounds the silence between two reads of a response body. A
	// whole-request deadline would abort large tarballs on slow links, so only a
	// stalled transfer is treated as a failure.
	stallTimeout = 60 * time.Second
)

// errStalled reports a response body that stopped delivering data.
var errStalled = fmt.Errorf("download stalled: no data received for %s", stallTimeout)

// Client performs GitHub HTTP requests. The zero value is not usable; use New.
type Client struct {
	http  *http.Client
	token string
	stall time.Duration
	warn  func(string)

	// Per-run caches so list and recursive runs touching the same repo do not
	// repeat API calls (the anonymous limit is 60 requests per hour).
	mu      sync.Mutex
	refs    map[string]refResult
	lists   map[string]map[string]string
	trees   map[string]treeResponse
	gitRefs map[string]advertisement

	// limited is set once the API refuses a request for rate limiting; from
	// then on refs come from the git HTTP endpoint and content from codeload,
	// neither of which counts against the API quota.
	limited bool
}

type refResult struct {
	sha string
	ok  bool
}

// New returns a Client. token may be empty for anonymous access to public repos.
// warn (optional) receives non-fatal notices, such as switching to the
// API-free fallback after hitting the rate limit.
func New(token string, warn func(string)) *Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = headerTimeout
	return &Client{
		http:    &http.Client{Transport: t},
		token:   token,
		stall:   stallTimeout,
		warn:    warn,
		refs:    map[string]refResult{},
		lists:   map[string]map[string]string{},
		trees:   map[string]treeResponse{},
		gitRefs: map[string]advertisement{},
	}
}

// isLimited reports whether the API has already rate-limited this client.
func (c *Client) isLimited() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.limited
}

// noteLimit reports whether err is an API rate limit and, the first time it
// is, switches the client to the API-free fallback and warns once.
func (c *Client) noteLimit(err error) bool {
	if !isRateLimit(err) {
		return false
	}
	c.mu.Lock()
	first := !c.limited
	c.limited = true
	c.mu.Unlock()
	if first && c.warn != nil {
		c.warn("GitHub API rate limit reached; continuing without the API " +
			"(whole-repo tarballs). Set GITHUB_TOKEN or log in with gh to avoid this")
	}
	return true
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
// The body is guarded by a stall watchdog instead of an overall deadline.
func (c *Client) do(rawurl, accept string) (*http.Response, error) {
	return c.send(rawurl, func(h http.Header) {
		h.Set("X-GitHub-Api-Version", "2022-11-28")
		if accept != "" {
			h.Set("Accept", accept)
		}
		if c.token != "" {
			h.Set("Authorization", "Bearer "+c.token)
		}
	})
}

// send issues a GET with the User-Agent plus the headers set by header, and
// wraps the body in the stall watchdog.
func (c *Client) send(rawurl string, header func(http.Header)) (*http.Response, error) {
	ctx, cancel := context.WithCancelCause(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawurl, nil)
	if err != nil {
		cancel(nil)
		return nil, err
	}
	req.Header.Set("User-Agent", "gh-get")
	header(req.Header)
	resp, err := c.http.Do(req)
	if err != nil {
		cancel(nil)
		return nil, err
	}
	resp.Body = newStallBody(ctx, resp.Body, c.stall, cancel)
	return resp, nil
}

// stallBody cancels its request when no data arrives for the given duration.
type stallBody struct {
	io.ReadCloser
	ctx    context.Context
	timer  *time.Timer
	stall  time.Duration
	cancel context.CancelCauseFunc
}

func newStallBody(ctx context.Context, body io.ReadCloser, stall time.Duration, cancel context.CancelCauseFunc) *stallBody {
	return &stallBody{
		ReadCloser: body,
		ctx:        ctx,
		timer:      time.AfterFunc(stall, func() { cancel(errStalled) }),
		stall:      stall,
		cancel:     cancel,
	}
}

func (b *stallBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.timer.Reset(b.stall)
	}
	if err != nil && errors.Is(context.Cause(b.ctx), errStalled) {
		err = errStalled
	}
	return n, err
}

func (b *stallBody) Close() error {
	b.timer.Stop()
	err := b.ReadCloser.Close()
	b.cancel(nil)
	return err
}

// getJSON fetches rawurl and returns the body bytes, mapping 403/429 to a
// rateLimitError and other non-2xx codes to a generic error.
func (c *Client) get(rawurl, accept string) ([]byte, error) {
	body, _, err := c.getWithHeader(rawurl, accept)
	return body, err
}

// getWithHeader is get that also returns the response headers.
func (c *Client) getWithHeader(rawurl, accept string) ([]byte, http.Header, error) {
	resp, err := c.do(rawurl, accept)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		return nil, nil, rateLimitError{status: resp.StatusCode}
	case resp.StatusCode == http.StatusNotFound:
		return nil, nil, fmt.Errorf("not found (HTTP 404): %s", rawurl)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, nil, fmt.Errorf("unexpected status HTTP %d for %s", resp.StatusCode, rawurl)
	}
	body, err := io.ReadAll(resp.Body)
	return body, resp.Header, err
}
