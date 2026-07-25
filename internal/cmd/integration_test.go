package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// capturedRequest records what a fake API server received, so a test can assert on
// the exact request body a write command produced.
type capturedRequest struct {
	method string
	path   string
	body   map[string]any
}

// runCLI runs the CLI against a fake API server with a temp config that already holds
// a valid token (so no refresh hits the real reddit.com), and returns stdout.
func runCLI(t *testing.T, server *httptest.Server, stdin string, args ...string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.json"), `{
		"app_id":"id","app_secret":"secret","refresh_token":"RT",
		"default_ad_account_id":"acc_1","user_agent":"test"
	}`)
	// A far-future expiry keeps ensureToken from refreshing.
	writeFile(t, filepath.Join(dir, "token.json"), `{"access_token":"AT","expires_at":"2999-01-01T00:00:00Z"}`)

	t.Setenv("REDDIT_ADS_BASE_URL", server.URL)

	var out bytes.Buffer
	app := &App{out: &out, in: strings.NewReader(stdin)}
	root := app.rootCmd()
	root.SetArgs(append([]string{"--config", dir}, args...))
	err := root.Execute()
	return out.String(), err
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCampaignsListJSONIsRawPassthrough(t *testing.T) {
	// The fake campaign carries created_at, a field the CLI's struct does not model.
	// --json must still surface it: it is a verbatim passthrough, not a re-serialized
	// subset.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ad_accounts/acc_1/campaigns" {
			_, _ = io.WriteString(w, `{"data":[{"id":"cmp_1","name":"Test","configured_status":"ACTIVE","created_at":"2026-01-01T00:00:00Z"}],"pagination":{}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	out, err := runCLI(t, srv, "", "campaigns", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "created_at") {
		t.Errorf("--json dropped an unmodeled field; got:\n%s", out)
	}
}

func TestCampaignsBudgetSendsMicrocurrency(t *testing.T) {
	captured := make(chan capturedRequest, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		captured <- capturedRequest{r.Method, r.URL.Path, body}
		_, _ = io.WriteString(w, `{"data":{"id":"cmp_1"}}`)
	}))
	defer srv.Close()

	// $50.00 must be sent as 50000000 micros. A 1e6 error here is a real budget bug.
	_, err := runCLI(t, srv, "", "campaigns", "budget", "cmp_1", "--spend-cap", "50", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	got := <-captured
	if got.method != http.MethodPatch || got.path != "/campaigns/cmp_1" {
		t.Fatalf("wrong request: %s %s", got.method, got.path)
	}
	data, _ := got.body["data"].(map[string]any)
	if data["spend_cap"].(float64) != 50000000 {
		t.Errorf("spend_cap = %v, want 50000000 micros", data["spend_cap"])
	}
}

func TestTargetingAddCommunityPreservesSiblingsOverTheWire(t *testing.T) {
	captured := make(chan capturedRequest, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/ad_groups/ag_1" {
			_, _ = io.WriteString(w, `{"data":{"id":"ag_1","targeting":{"gender":["FEMALE"],"communities":["t5_old"]}}}`)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		captured <- capturedRequest{r.Method, r.URL.Path, body}
		_, _ = io.WriteString(w, `{"data":{"id":"ag_1"}}`)
	}))
	defer srv.Close()

	_, err := runCLI(t, srv, "", "adgroups", "targeting", "add-community", "ag_1", "t5_new", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	got := <-captured
	data := got.body["data"].(map[string]any)
	targeting := data["targeting"].(map[string]any)

	if _, ok := targeting["gender"]; !ok {
		t.Error("read-modify-write dropped the gender sibling field")
	}
	comms := targeting["communities"].([]any)
	if len(comms) != 2 {
		t.Errorf("communities = %v, want old + new", comms)
	}
}

func TestReportTableConvertsSpendToDollars(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/ad_accounts/acc_1/reports" {
			_, _ = io.WriteString(w, `{"data":{"metrics":[{"impressions":1000,"clicks":10,"spend":1500000,"cpc":150000}]}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	out, err := runCLI(t, srv, "", "report", "--from", "2026-07-01", "--to", "2026-07-25",
		"--fields", "IMPRESSIONS,CLICKS,SPEND,CPC")
	if err != nil {
		t.Fatal(err)
	}
	// 1500000 micros is $1.50; a table showing 1500000 would mean no conversion.
	if !strings.Contains(out, "1.50") {
		t.Errorf("spend not converted to dollars; got:\n%s", out)
	}
	if strings.Contains(out, "1500000") {
		t.Errorf("raw microcurrency leaked into the table; got:\n%s", out)
	}
}

func TestAuthLoginWizardPromptsExchangesAndPersists(t *testing.T) {
	// Fake token endpoint returns a refresh token for any code.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("code") != "the-code" {
			t.Errorf("exchange sent code %q, want the-code", r.Form.Get("code"))
		}
		_, _ = io.WriteString(w, `{"access_token":"AT","refresh_token":"RT","expires_in":3600}`)
	}))
	defer tokenSrv.Close()

	dir := t.TempDir()
	t.Setenv("REDDIT_ADS_TOKEN_URL", tokenSrv.URL)

	// Wizard prompts in order: app id, secret, redirect (blank = default),
	// then the pasted redirect URL that carries the code.
	stdin := "cid\nsecret\n\nhttps://cb/callback?code=the-code#_\n"
	var out bytes.Buffer
	app := &App{out: &out, in: strings.NewReader(stdin)}
	root := app.rootCmd()
	root.SetArgs([]string{"--config", dir, "auth", "login"})
	if err := root.Execute(); err != nil {
		t.Fatalf("wizard failed: %v\noutput:\n%s", err, out.String())
	}

	// The credentials and refresh token must be persisted so login is a one-time thing.
	saved := readJSON(t, filepath.Join(dir, "config.json"))
	if saved["app_id"] != "cid" || saved["app_secret"] != "secret" {
		t.Errorf("credentials not persisted: %v", saved)
	}
	if saved["refresh_token"] != "RT" {
		t.Errorf("refresh token not persisted: %v", saved)
	}
	if saved["redirect_uri"] != defaultRedirectURI {
		t.Errorf("redirect uri = %v, want the default that was accepted with an empty line", saved["redirect_uri"])
	}
}

func TestAuthLoginCapturesCodeViaLoopback(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("code") != "the-code" {
			t.Errorf("exchange sent code %q, want the-code", r.Form.Get("code"))
		}
		_, _ = io.WriteString(w, `{"access_token":"AT","refresh_token":"RT","expires_in":3600}`)
	}))
	defer tokenSrv.Close()

	// Reserve a free port, then hand it to the CLI as the redirect so the loopback
	// server binds it. Small TOCTOU window, fine for a test.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	freePort := probe.Addr().(*net.TCPAddr).Port
	probe.Close()
	redirect := fmt.Sprintf("http://127.0.0.1:%d/reddit-ads/callback", freePort)

	dir := t.TempDir()
	t.Setenv("REDDIT_ADS_TOKEN_URL", tokenSrv.URL)

	var out bytes.Buffer
	// Empty stdin: no paste, so the browser redirect is what completes the login.
	app := &App{out: &out, in: strings.NewReader("")}
	root := app.rootCmd()
	// Pin --state so the simulated callback below carries a matching value (the default
	// is now a random per-login nonce).
	root.SetArgs([]string{"--config", dir, "auth", "login",
		"--app-id", "APPID", "--app-secret", "SECRET", "--redirect-uri", redirect, "--state", "test-state"})

	done := make(chan error, 1)
	go func() { done <- root.Execute() }()

	// Simulate the browser landing on the redirect, retrying until the server is up.
	callback := fmt.Sprintf("%s?code=the-code&state=test-state", redirect)
	var page string
	for i := 0; i < 200; i++ {
		resp, err := http.Get(callback)
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			page = string(b)
			break
		}
		time.Sleep(15 * time.Millisecond)
	}

	if err := <-done; err != nil {
		t.Fatalf("login failed: %v\n%s", err, out.String())
	}
	if !strings.Contains(strings.ToLower(page), "successful") {
		t.Errorf("callback page did not confirm success: %s", page)
	}
	if !strings.Contains(out.String(), "Authentication successful") {
		t.Errorf("terminal did not report success:\n%s", out.String())
	}
	saved := readJSON(t, filepath.Join(dir, "config.json"))
	if saved["refresh_token"] != "RT" {
		t.Errorf("refresh token not persisted after loopback capture: %v", saved)
	}
}

func TestAuthLoginPersistsCredentialsFromFlags(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The App ID is sent to Reddit as the OAuth client id (HTTP Basic username).
		user, _, _ := r.BasicAuth()
		if user != "APPID" {
			t.Errorf("app id not sent as the OAuth basic-auth user: got %q", user)
		}
		_, _ = io.WriteString(w, `{"access_token":"AT","refresh_token":"RT","expires_in":3600}`)
	}))
	defer tokenSrv.Close()

	dir := t.TempDir()
	t.Setenv("REDDIT_ADS_TOKEN_URL", tokenSrv.URL)

	// Credentials come from flags; only the pasted code is prompted. Redirect "https://cb/x"
	// can't bind a loopback, so it falls back to the pasted code cleanly.
	var out bytes.Buffer
	app := &App{out: &out, in: strings.NewReader("the-code\n")}
	root := app.rootCmd()
	root.SetArgs([]string{"--config", dir, "auth", "login",
		"--app-id", "APPID", "--app-secret", "SECRET", "--redirect-uri", "https://cb/x"})
	if err := root.Execute(); err != nil {
		t.Fatalf("login with flags failed: %v\n%s", err, out.String())
	}
	saved := readJSON(t, filepath.Join(dir, "config.json"))
	if saved["app_id"] != "APPID" || saved["app_secret"] != "SECRET" {
		t.Errorf("flags did not persist credentials as app_id/app_secret: %v", saved)
	}
}

func TestAdsClickURLSendsQueryParamArray(t *testing.T) {
	captured := make(chan capturedRequest, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		captured <- capturedRequest{r.Method, r.URL.Path, body}
		_, _ = io.WriteString(w, `{"data":{"id":"ad_1"}}`)
	}))
	defer srv.Close()

	// Reddit requires an array of {name,value} objects here, not a raw query string.
	_, err := runCLI(t, srv, "", "ads", "click-url", "ad_1", "--query-params", "utm_source=reddit&utm_medium=cpc", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	req := <-captured
	data, _ := req.body["data"].(map[string]any)
	params, ok := data["click_url_query_parameters"].([]any)
	if !ok || len(params) != 2 {
		t.Fatalf("click_url_query_parameters not sent as a 2-element array: %#v", data["click_url_query_parameters"])
	}
	first, _ := params[0].(map[string]any)
	if first["name"] != "utm_source" || first["value"] != "reddit" {
		t.Errorf("first query param = %#v, want name=utm_source value=reddit", first)
	}
}

func TestReportRunsUnderDryRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"metrics":[{"impressions":100,"spend":500000}]}}`)
	}))
	defer srv.Close()

	// A report is a read modeled as POST, so --dry-run must still return data.
	out, err := runCLI(t, srv, "", "report", "--from", "2026-07-01", "--to", "2026-07-02",
		"--fields", "IMPRESSIONS,SPEND", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "DRY RUN") {
		t.Errorf("report under --dry-run should run, not print a dry-run stub:\n%s", out)
	}
	if !strings.Contains(out, "0.50") {
		t.Errorf("report under --dry-run did not render metrics (spend 0.50):\n%s", out)
	}
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestWriteAbortsWhenUserDeclinesConfirmation(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = io.WriteString(w, `{"data":{}}`)
	}))
	defer srv.Close()

	// Answering "n" (no --yes) must send nothing.
	_, err := runCLI(t, srv, "n\n", "campaigns", "pause", "cmp_1")
	if err != nil {
		t.Fatal(err)
	}
	if hits != 0 {
		t.Errorf("declined confirmation still sent %d requests", hits)
	}
}
