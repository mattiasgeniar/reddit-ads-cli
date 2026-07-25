package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mattiasgeniar/reddit-ads-cli/internal/config"
)

// DefaultBaseURL is the Reddit Ads API v3 root.
const DefaultBaseURL = "https://ads-api.reddit.com/api/v3"

// tokenSkew is how far before real expiry we treat a token as needing refresh.
const tokenSkew = 60 * time.Second

// Client talks to the Reddit Ads API. It refreshes the access token on demand,
// follows next_url pagination, backs off on 429, and can short-circuit any write as
// a --dry-run that prints the request instead of sending it.
type Client struct {
	cfg   *config.Config
	http  *http.Client
	base  string
	token *config.Token

	// DryRun makes mutating requests (anything but GET) print and return without
	// sending. Reads still go through so a dry-run write can read-modify-write.
	DryRun bool
	// Out is where dry-run output and diagnostics are written.
	Out io.Writer

	maxRetries int
}

// New builds a client from config. It loads any cached token but does not refresh
// until the first request needs it.
func New(cfg *config.Config, out io.Writer) (*Client, error) {
	tok, err := cfg.LoadToken()
	if err != nil {
		return nil, err
	}
	base := DefaultBaseURL
	if cfg.BaseURL != "" {
		base = strings.TrimRight(cfg.BaseURL, "/")
	}
	return &Client{
		cfg:        cfg,
		http:       &http.Client{Timeout: 60 * time.Second},
		base:       base,
		token:      tok,
		Out:        out,
		maxRetries: 4,
	}, nil
}

// APIError is a non-2xx response from the ads API, carrying the status and body so
// callers can show Reddit's own message.
type APIError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *APIError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("reddit ads api: %s", e.Status)
	}
	return fmt.Sprintf("reddit ads api: %s: %s", e.Status, e.Body)
}

// ensureToken makes sure c.token is valid, refreshing and persisting it if not.
func (c *Client) ensureToken() error {
	if c.token.Valid(tokenSkew) {
		return nil
	}
	tok, err := RefreshAccessToken(c.cfg)
	if err != nil {
		return err
	}
	c.token = tok
	return c.cfg.SaveToken(tok)
}

// Get issues a GET and decodes the JSON body into out (out may be nil to discard).
func (c *Client) Get(path string, out any) error {
	return c.doJSON(http.MethodGet, path, nil, out, false)
}

// postRead issues a POST that is semantically a read (a report query), so it is not
// gated by --dry-run: a dry-run report should still return data, like a GET.
func (c *Client) postRead(path string, body, out any) error {
	return c.doJSON(http.MethodPost, path, body, out, false)
}

// GetRaw issues a GET and returns the response body verbatim, for the --json
// passthrough that must mirror exactly what Reddit sent.
func (c *Client) GetRaw(path string) (json.RawMessage, error) {
	resp, err := c.do(http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{StatusCode: resp.StatusCode, Status: resp.Status, Body: strings.TrimSpace(string(body))}
	}
	return body, nil
}

// Post issues a POST with a JSON body and decodes the response into out.
func (c *Client) Post(path string, body, out any) error {
	return c.doJSON(http.MethodPost, path, body, out, true)
}

// Patch issues a PATCH with a JSON body and decodes the response into out.
func (c *Client) Patch(path string, body, out any) error {
	return c.doJSON(http.MethodPatch, path, body, out, true)
}

// doJSON marshals body, sends the request, and decodes the response into out. When
// mutating is true, --dry-run prints the request and returns without sending it; reads
// (mutating false) always go through, so a dry-run write can still read-modify-write.
func (c *Client) doJSON(method, path string, body, out any, mutating bool) error {
	var raw []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		raw = b
	}

	if c.DryRun && mutating {
		c.printDryRun(method, path, raw)
		return nil
	}

	resp, err := c.do(method, c.base+path, raw)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Status: resp.Status, Body: strings.TrimSpace(string(respBody))}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(respBody, out)
}

// do performs a single request with auth, retrying on 429 (honoring the reset value)
// and once on 401 after a forced token refresh. url is absolute.
func (c *Client) do(method, url string, raw []byte) (*http.Response, error) {
	if err := c.ensureToken(); err != nil {
		return nil, err
	}

	refreshedOn401 := false
	for attempt := 0; ; attempt++ {
		var reqBody io.Reader
		if raw != nil {
			reqBody = bytes.NewReader(raw)
		}
		req, err := http.NewRequest(method, url, reqBody)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.token.AccessToken)
		req.Header.Set("User-Agent", c.cfg.UserAgent)
		req.Header.Set("Accept", "application/json")
		if raw != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}

		// A 401 usually means the access token went stale between the skew check and
		// the request. Force one refresh and retry.
		if resp.StatusCode == http.StatusUnauthorized && !refreshedOn401 {
			resp.Body.Close()
			refreshedOn401 = true
			c.token.ExpiresAt = time.Time{} // invalidate so ensureToken refreshes
			if err := c.ensureToken(); err != nil {
				return nil, err
			}
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < c.maxRetries {
			wait := c.backoff(resp, attempt)
			resp.Body.Close()
			c.logf("rate limited, waiting %s before retry %d/%d", wait, attempt+1, c.maxRetries)
			time.Sleep(wait)
			continue
		}

		return resp, nil
	}
}

// maxBackoff caps how long a single 429 wait can be, so a bogus or huge reset value
// from an edge/proxy cannot stall a command for minutes. Reddit's real windows are ~60s.
const maxBackoff = 2 * time.Minute

// backoff decides how long to wait after a 429. It prefers the reset seconds Reddit
// reports (IETF RateLimit header or Retry-After) and only falls back to exponential
// backoff when no header is present. The result is capped at maxBackoff.
func (c *Client) backoff(resp *http.Response, attempt int) time.Duration {
	wait := time.Duration(1<<attempt) * time.Second // 1s, 2s, 4s, 8s...
	if secs, ok := resetSeconds(resp.Header); ok && secs > 0 {
		wait = time.Duration(secs) * time.Second
	}
	if wait > maxBackoff {
		return maxBackoff
	}
	return wait
}

// resetSeconds pulls the seconds-until-reset out of Reddit's rate-limit headers.
// It accepts the IETF structured `RateLimit` header, a bare `Retry-After: N`, and the
// legacy `x-ratelimit-reset` variants.
func resetSeconds(h http.Header) (int, bool) {
	if v := h.Get("Retry-After"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n, true
		}
	}
	if v := h.Get("RateLimit"); v != "" {
		if n, ok := parseStructuredReset(v); ok {
			return n, true
		}
	}
	for _, key := range []string{"X-Ratelimit-Reset", "x-ratelimit-reset"} {
		if v := h.Get(key); v != "" {
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				return int(f), true
			}
		}
	}
	return 0, false
}

// parseStructuredReset extracts the seconds-until-reset from the IETF RateLimit header.
// The reset is carried as `t=N`, in either the newer semicolon form
// (`"default";r=0;t=30`) or a comma-separated form (`limit=100, remaining=0, t=30`).
// The older draft spelled it `reset=N`, so accept that too, preferring `t`.
func parseStructuredReset(v string) (int, bool) {
	var resetFallback int
	haveFallback := false
	fields := strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ';' })
	for _, part := range fields {
		part = strings.TrimSpace(part)
		if after, ok := strings.CutPrefix(part, "t="); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(after)); err == nil {
				return n, true
			}
		}
		if after, ok := strings.CutPrefix(part, "reset="); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(after)); err == nil {
				resetFallback, haveFallback = n, true
			}
		}
	}
	return resetFallback, haveFallback
}

func (c *Client) printDryRun(method, path string, raw []byte) {
	fmt.Fprintf(c.Out, "DRY RUN: would send\n  %s %s%s\n", method, c.base, path)
	if len(raw) > 0 {
		var pretty bytes.Buffer
		if json.Indent(&pretty, raw, "  ", "  ") == nil {
			fmt.Fprintf(c.Out, "  body:\n  %s\n", pretty.String())
			return
		}
		fmt.Fprintf(c.Out, "  body: %s\n", raw)
	}
}

func (c *Client) logf(format string, args ...any) {
	if c.Out != nil {
		fmt.Fprintf(c.Out, format+"\n", args...)
	}
}
