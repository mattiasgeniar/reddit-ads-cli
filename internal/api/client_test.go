package api

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mattiasgeniar/reddit-ads-cli/internal/config"
)

// testClient returns a Client pointed at base with an already-valid token so no
// refresh (and thus no real network to reddit.com) is attempted.
func testClient(base string) *Client {
	return &Client{
		cfg:        &config.Config{UserAgent: "test"},
		http:       &http.Client{Timeout: 5 * time.Second},
		base:       base,
		token:      &config.Token{AccessToken: "valid", ExpiresAt: time.Now().Add(time.Hour)},
		Out:        io.Discard,
		maxRetries: 4,
	}
}

func TestGetListFollowsNextURL(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Page 2 arrives as the verbatim next_url, distinguished by its query string.
		if r.URL.RawQuery == "page=2" {
			fmt.Fprint(w, `{"data":[{"id":"c"}],"pagination":{}}`)
			return
		}
		fmt.Fprintf(w, `{"data":[{"id":"a"},{"id":"b"}],"pagination":{"next_url":%q}}`, srv.URL+"/things?page=2")
	}))
	defer srv.Close()

	c := testClient(srv.URL)
	got, err := c.GetList("/things")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d elements across pages, want 3: %v", len(got), got)
	}
}

func TestDoRetriesOn429WithRetryAfter(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, `{"data":{"ok":true}}`)
	}))
	defer srv.Close()

	c := testClient(srv.URL)
	var out struct {
		Data struct {
			OK bool `json:"ok"`
		} `json:"data"`
	}
	if err := c.Get("/thing", &out); err != nil {
		t.Fatal(err)
	}
	if !out.Data.OK {
		t.Error("expected successful retry after 429")
	}
	if calls.Load() != 2 {
		t.Errorf("expected 2 calls (429 then 200), got %d", calls.Load())
	}
}

func TestDryRunDoesNotSendWrites(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
	}))
	defer srv.Close()

	c := testClient(srv.URL)
	c.DryRun = true
	if err := c.Patch("/campaigns/1", map[string]any{"data": map[string]any{"configured_status": "PAUSED"}}, nil); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Errorf("dry-run must not send a request, got %d calls", calls.Load())
	}
}

func TestDryRunStillAllowsReads(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"id":"x"}}`)
	}))
	defer srv.Close()

	c := testClient(srv.URL)
	c.DryRun = true
	var out map[string]any
	if err := c.Get("/campaigns/1", &out); err != nil {
		t.Fatalf("dry-run GET should still send: %v", err)
	}
}

func TestParseStructuredReset(t *testing.T) {
	// Legacy comma-separated reset= form.
	if n, ok := parseStructuredReset("limit=100, remaining=0, reset=42"); !ok || n != 42 {
		t.Errorf("parseStructuredReset(reset=) = %d,%v want 42,true", n, ok)
	}
	// IETF structured form carries the reset as t=, semicolon-delimited. This is what
	// Reddit actually sends; the old parser missed it and ignored the backoff signal.
	if n, ok := parseStructuredReset(`"default";r=0;t=30`); !ok || n != 30 {
		t.Errorf("parseStructuredReset(;t=) = %d,%v want 30,true", n, ok)
	}
	// t is preferred over reset when both are present.
	if n, ok := parseStructuredReset("reset=99, t=7"); !ok || n != 7 {
		t.Errorf("parseStructuredReset prefers t: = %d,%v want 7,true", n, ok)
	}
}

func TestResetSecondsReadsIETFRateLimitHeader(t *testing.T) {
	h := http.Header{}
	h.Set("RateLimit", `"campaign-read";r=0;t=25`)
	if secs, ok := resetSeconds(h); !ok || secs != 25 {
		t.Errorf("resetSeconds from RateLimit header = %d,%v want 25,true", secs, ok)
	}
}
