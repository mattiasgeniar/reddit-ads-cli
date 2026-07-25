package cmd

import (
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

func loopbackGet(t *testing.T, query string) (body string, result loopbackResult) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv, ch := serveLoopback(ln, "/reddit-ads/callback", "st8")
	defer srv.Close()

	resp, err := http.Get("http://" + ln.Addr().String() + "/reddit-ads/callback?" + query)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return string(b), <-ch
}

func TestLoopbackCapturesCode(t *testing.T) {
	body, res := loopbackGet(t, "code=abc123&state=st8")
	if res.err != nil {
		t.Fatalf("unexpected error: %v", res.err)
	}
	if res.code != "abc123" {
		t.Errorf("captured code = %q, want abc123", res.code)
	}
	if !strings.Contains(strings.ToLower(body), "successful") {
		t.Errorf("success page missing confirmation, got: %s", body)
	}
}

func TestLoopbackRejectsStateMismatch(t *testing.T) {
	_, res := loopbackGet(t, "code=abc123&state=WRONG")
	if res.err == nil {
		t.Error("expected an error on state mismatch (CSRF guard)")
	}
	if res.code != "" {
		t.Errorf("must not return a code on state mismatch, got %q", res.code)
	}
}

func TestLoopbackReportsProviderError(t *testing.T) {
	_, res := loopbackGet(t, "error=access_denied&state=st8")
	if res.err == nil {
		t.Error("expected an error when Reddit returns error=access_denied")
	}
}

func TestWithPort(t *testing.T) {
	got, err := withPort("http://localhost:8080/reddit-ads/callback", 9123)
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://localhost:9123/reddit-ads/callback" {
		t.Errorf("withPort = %q", got)
	}
}
