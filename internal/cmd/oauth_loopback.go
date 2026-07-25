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
// A bind failure (port taken) or a non-loopback host is returned so the caller can fall
// back to paste. It refuses any non-loopback host so the capture endpoint is never
// exposed to the LAN (e.g. a redirect of http://0.0.0.0:8080 or an empty host).
func startLoopback(redirectURI, expectedState string) (*loopbackServer, error) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return nil, err
	}
	if !isLoopbackHost(u.Hostname()) {
		return nil, fmt.Errorf("redirect host %q is not loopback; refusing to bind a network-facing capture server", u.Hostname())
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
	// The result is consumed exactly once, so a second concurrent callback (a double
	// submit) must not block its handler goroutine forever: send without blocking.
	send := func(r loopbackResult) {
		select {
		case ch <- r:
		default:
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		if e := q.Get("error"); e != "" {
			writeCallbackPage(w, false, "Reddit returned an error: "+e)
			send(loopbackResult{err: fmt.Errorf("authorization failed: %s", e)})
			return
		}
		// state guards against a cross-site request forging the callback.
		if expectedState != "" && q.Get("state") != expectedState {
			writeCallbackPage(w, false, "State did not match. Ignoring this redirect.")
			send(loopbackResult{err: fmt.Errorf("state mismatch on the OAuth redirect")})
			return
		}
		code := q.Get("code")
		if code == "" {
			writeCallbackPage(w, false, "No code was present in the redirect.")
			send(loopbackResult{err: fmt.Errorf("no code in the redirect")})
			return
		}
		writeCallbackPage(w, true, "")
		send(loopbackResult{code: code})
	})

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	return srv, ch
}

// isLoopbackHost reports whether host is a loopback address (127.0.0.0/8, ::1, or the
// name "localhost"). An empty or non-loopback host returns false.
func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
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
