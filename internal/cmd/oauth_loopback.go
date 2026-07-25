package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// loopbackResult is what the local callback server captures: an OAuth code, or an
// error (denied authorization, state mismatch, missing code).
type loopbackResult struct {
	code string
	err  error
}

// loopbackServer is a one-shot local HTTP server that catches the OAuth redirect so the
// user never has to copy the code by hand. It only works when the browser and the CLI
// are on the same machine; when they are not (a remote box over SSH), the caller falls
// back to pasting, so this stays best-effort.
type loopbackServer struct {
	srv    *http.Server
	result <-chan loopbackResult
	addr   string
}

// startLoopback binds a server on the redirect URI's host and port and serves its path.
// A bind failure (the port is taken) is returned so the caller can fall back to paste.
func startLoopback(redirectURI, expectedState string) (*loopbackServer, error) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return nil, err
	}
	port := u.Port()
	if port == "" {
		port = "80"
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(u.Hostname(), port))
	if err != nil {
		return nil, err
	}

	path := u.Path
	if path == "" {
		path = "/"
	}
	srv, ch := serveLoopback(ln, path, expectedState)
	return &loopbackServer{srv: srv, result: ch, addr: ln.Addr().String()}, nil
}

// serveLoopback wires the callback handler onto an already-open listener. Split out so
// tests can drive it on an ephemeral port without racing on a fixed one.
func serveLoopback(ln net.Listener, path, expectedState string) (*http.Server, <-chan loopbackResult) {
	ch := make(chan loopbackResult, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		if e := q.Get("error"); e != "" {
			writeCallbackPage(w, false, "Reddit returned an error: "+e)
			ch <- loopbackResult{err: fmt.Errorf("authorization failed: %s", e)}
			return
		}
		// state guards against a cross-site request forging the callback.
		if expectedState != "" && q.Get("state") != expectedState {
			writeCallbackPage(w, false, "State did not match. Ignoring this redirect.")
			ch <- loopbackResult{err: fmt.Errorf("state mismatch on the OAuth redirect")}
			return
		}
		code := q.Get("code")
		if code == "" {
			writeCallbackPage(w, false, "No code was present in the redirect.")
			ch <- loopbackResult{err: fmt.Errorf("no code in the redirect")}
			return
		}
		writeCallbackPage(w, true, "")
		ch <- loopbackResult{code: code}
	})

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	return srv, ch
}

// shutdown drains the server so the browser finishes rendering the success page before
// the process exits.
func (l *loopbackServer) shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = l.srv.Shutdown(ctx)
}

// writeCallbackPage renders the tab the user is left looking at after the redirect.
func writeCallbackPage(w http.ResponseWriter, ok bool, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	title, body := "Authentication successful", "reddit-ads captured the redirect. You can close this tab and return to your terminal."
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		title, body = "Authentication failed", message
	}
	fmt.Fprintf(w, `<!doctype html><html><head><meta charset="utf-8"><title>%s</title>
<style>body{font-family:system-ui,sans-serif;max-width:34rem;margin:12vh auto;padding:0 1.5rem;color:#111}
h1{font-size:1.4rem;margin:0 0 .5rem}p{color:#444;line-height:1.5}.d{color:#d33}</style></head>
<body><h1 class="%s">%s</h1><p>%s</p></body></html>`,
		title, map[bool]string{true: "", false: "d"}[ok], title, body)
}
