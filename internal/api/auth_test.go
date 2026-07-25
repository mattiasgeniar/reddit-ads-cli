package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mattiasgeniar/reddit-ads-cli/internal/config"
)

func assertEqual(t *testing.T, label, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %q, want %q", label, got, want)
	}
}

func TestAuthorizeURLShape(t *testing.T) {
	raw := AuthorizeURL("CID", "https://cb.example/redirect", "st8", V1Scopes)
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(raw, authorizeURL) {
		t.Errorf("authorize URL host wrong: %s", raw)
	}
	q := u.Query()
	assertEqual(t, "client_id", q.Get("client_id"), "CID")
	assertEqual(t, "response_type", q.Get("response_type"), "code")
	assertEqual(t, "duration", q.Get("duration"), "permanent")
	assertEqual(t, "scope", q.Get("scope"), "adsread,adsedit")
	assertEqual(t, "redirect_uri", q.Get("redirect_uri"), "https://cb.example/redirect")
	assertEqual(t, "state", q.Get("state"), "st8")
}

func TestExchangeCodeUsesBasicAuthAndReturnsRefreshToken(t *testing.T) {
	var gotAuthUser, gotAuthPass, gotGrant, gotCode string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthUser, gotAuthPass, _ = r.BasicAuth()
		_ = r.ParseForm()
		gotGrant = r.Form.Get("grant_type")
		gotCode = r.Form.Get("code")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"AT","refresh_token":"RT","expires_in":3600,"scope":"adsread adsedit"}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AppID: "id", AppSecret: "secret", UserAgent: "test", TokenURL: srv.URL}
	refresh, token, err := ExchangeCode(cfg, "the-code", "https://cb.example/redirect")
	if err != nil {
		t.Fatal(err)
	}
	assertEqual(t, "basic user", gotAuthUser, "id")
	assertEqual(t, "basic pass", gotAuthPass, "secret")
	assertEqual(t, "grant", gotGrant, "authorization_code")
	assertEqual(t, "code", gotCode, "the-code")
	assertEqual(t, "refresh token", refresh, "RT")
	assertEqual(t, "access token", token.AccessToken, "AT")
	// expires_in 3600 minus a minute of slack should land well in the future.
	if !token.ExpiresAt.After(time.Now().Add(5 * time.Minute)) {
		t.Errorf("token expiry %v should be ~1h out", token.ExpiresAt)
	}
}

func TestExchangeCodeRejectsMissingRefreshToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"AT","expires_in":3600}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AppID: "id", AppSecret: "secret", UserAgent: "test", TokenURL: srv.URL}
	if _, _, err := ExchangeCode(cfg, "code", "cb"); err == nil {
		t.Error("expected error when duration=permanent is missing and no refresh token returned")
	}
}

func TestRefreshAccessTokenSendsRefreshGrant(t *testing.T) {
	var gotGrant, gotRefresh string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotGrant = r.Form.Get("grant_type")
		gotRefresh = r.Form.Get("refresh_token")
		_, _ = w.Write([]byte(`{"access_token":"AT2","expires_in":3600}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AppID: "id", AppSecret: "secret", RefreshToken: "RT", UserAgent: "test", TokenURL: srv.URL}
	token, err := RefreshAccessToken(cfg)
	if err != nil {
		t.Fatal(err)
	}
	assertEqual(t, "grant", gotGrant, "refresh_token")
	assertEqual(t, "refresh_token sent", gotRefresh, "RT")
	assertEqual(t, "access token", token.AccessToken, "AT2")
}

func TestRefreshAccessTokenErrorsWithoutRefreshToken(t *testing.T) {
	cfg := &config.Config{AppID: "id", AppSecret: "secret", UserAgent: "test"}
	if _, err := RefreshAccessToken(cfg); err == nil {
		t.Error("expected error with no stored refresh token")
	}
}

func TestPostTokenSurfacesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AppID: "id", AppSecret: "secret", RefreshToken: "RT", UserAgent: "test", TokenURL: srv.URL}
	if _, err := RefreshAccessToken(cfg); err == nil {
		t.Error("expected error on 401 from token endpoint")
	}
}
