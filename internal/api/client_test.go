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
	n, ok := parseStructuredReset("limit=100, remaining=0, reset=42")
	if !ok || n != 42 {
		t.Errorf("parseStructuredReset = %d,%v want 42,true", n, ok)
	}
}
