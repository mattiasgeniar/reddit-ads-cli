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

	"github.com/mattiasgeniar/reddit-ads-cli/internal/config"
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

	// After login the CLI looks up ad accounts to pick a default; keep that offline.
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[],"pagination":{}}`)
	}))
	defer apiSrv.Close()

	dir := t.TempDir()
	t.Setenv("REDDIT_ADS_TOKEN_URL", tokenSrv.URL)
	t.Setenv("REDDIT_ADS_BASE_URL", apiSrv.URL)

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

	// After login the CLI looks up ad accounts to pick a default; keep that offline.
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[],"pagination":{}}`)
	}))
	defer apiSrv.Close()

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
	t.Setenv("REDDIT_ADS_BASE_URL", apiSrv.URL)

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

	// After login the CLI looks up ad accounts to pick a default; keep that offline.
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[],"pagination":{}}`)
	}))
	defer apiSrv.Close()

	dir := t.TempDir()
	t.Setenv("REDDIT_ADS_TOKEN_URL", tokenSrv.URL)
	t.Setenv("REDDIT_ADS_BASE_URL", apiSrv.URL)

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

// authedApp builds an App with a valid cached token pointed at baseURL, for exercising
// post-login steps like default-account resolution directly.
func authedApp(t *testing.T, baseURL, stdin string, out *bytes.Buffer) *App {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.json"), `{"app_id":"id","app_secret":"secret","refresh_token":"RT","user_agent":"test"}`)
	writeFile(t, filepath.Join(dir, "token.json"), `{"access_token":"AT","expires_at":"2999-01-01T00:00:00Z"}`)
	t.Setenv("REDDIT_ADS_BASE_URL", baseURL)
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return &App{cfg: cfg, out: out, in: strings.NewReader(stdin)}
}

// accountsServer serves fixed JSON per request path, for the businesses/ad_accounts fan-out.
func accountsServer(bodies map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if body, ok := bodies[r.URL.Path]; ok {
			_, _ = io.WriteString(w, body)
			return
		}
		http.NotFound(w, r)
	}))
}

func TestSetupDefaultAccountAutoSetsLoneAccount(t *testing.T) {
	srv := accountsServer(map[string]string{
		"/me/businesses":             `{"data":[{"id":"b1"}],"pagination":{}}`,
		"/businesses/b1/ad_accounts": `{"data":[{"id":"acc_solo","name":"Solo"}],"pagination":{}}`,
	})
	defer srv.Close()

	var out bytes.Buffer
	app := authedApp(t, srv.URL, "", &out)
	if err := app.setupDefaultAccount(app.prompter()); err != nil {
		t.Fatal(err)
	}
	if app.cfg.DefaultAdAccountID != "acc_solo" {
		t.Errorf("lone account not auto-set as default: %q", app.cfg.DefaultAdAccountID)
	}
	saved := readJSON(t, filepath.Join(app.cfg.Dir(), "config.json"))
	if saved["default_ad_account_id"] != "acc_solo" {
		t.Errorf("default not persisted to config: %v", saved)
	}
}

func TestSetupDefaultAccountPromptsAmongMany(t *testing.T) {
	srv := accountsServer(map[string]string{
		"/me/businesses":             `{"data":[{"id":"b1"}],"pagination":{}}`,
		"/businesses/b1/ad_accounts": `{"data":[{"id":"acc_1","name":"One"},{"id":"acc_2","name":"Two"}],"pagination":{}}`,
	})
	defer srv.Close()

	var out bytes.Buffer
	app := authedApp(t, srv.URL, "2\n", &out)
	if err := app.setupDefaultAccount(app.prompter()); err != nil {
		t.Fatal(err)
	}
	if app.cfg.DefaultAdAccountID != "acc_2" {
		t.Errorf("prompt pick did not select the 2nd account: %q", app.cfg.DefaultAdAccountID)
	}
	if !strings.Contains(out.String(), "acc_1") || !strings.Contains(out.String(), "acc_2") {
		t.Errorf("account choices were not listed:\n%s", out.String())
	}
}

func TestSetupDefaultAccountSkipsWhenNoChoice(t *testing.T) {
	srv := accountsServer(map[string]string{
		"/me/businesses":             `{"data":[{"id":"b1"}],"pagination":{}}`,
		"/businesses/b1/ad_accounts": `{"data":[{"id":"acc_1"},{"id":"acc_2"}],"pagination":{}}`,
	})
	defer srv.Close()

	var out bytes.Buffer
	app := authedApp(t, srv.URL, "", &out) // no input: skip, don't hang
	if err := app.setupDefaultAccount(app.prompter()); err != nil {
		t.Fatal(err)
	}
	if app.cfg.DefaultAdAccountID != "" {
		t.Errorf("no choice should leave the default unset, got %q", app.cfg.DefaultAdAccountID)
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

func TestReportFollowsEveryPageWithTheSameBody(t *testing.T) {
	var bodies []string
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/ad_accounts/acc_1/reports" {
			http.NotFound(w, r)
			return
		}
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if r.URL.Query().Get("page.token") == "" {
			fmt.Fprintf(w, `{"data":{"metrics":[{"clicks":2,"spend":1000000}]},"pagination":{"next_url":%q}}`,
				srv.URL+"/ad_accounts/acc_1/reports?page.token=p2")
			return
		}
		_, _ = io.WriteString(w, `{"data":{"metrics":[{"clicks":3,"spend":2000000}]},"pagination":{"next_url":null}}`)
	}))
	defer srv.Close()

	out, err := runCLI(t, srv, "", "report", "--from", "2026-10-09", "--to", "2026-10-10",
		"--by", "adgroup", "--fields", "CLICKS,SPEND", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Data struct {
			Metrics []struct {
				Clicks int `json:"clicks"`
			} `json:"metrics"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if len(got.Data.Metrics) != 2 || got.Data.Metrics[1].Clicks != 3 {
		t.Errorf("metrics = %+v, want the rows of both pages", got.Data.Metrics)
	}
	if len(bodies) != 2 || bodies[0] != bodies[1] {
		t.Errorf("page requests = %q, want two POSTs with the same body", bodies)
	}
}

func TestReportSendsTheExclusiveToAsRedditsEndsAt(t *testing.T) {
	var sent map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		sent = body["data"]
		_, _ = io.WriteString(w, `{"data":{"metrics":[]}}`)
	}))
	defer srv.Close()

	if _, err := runCLI(t, srv, "", "report", "--from", "2026-10-08", "--to", "2026-10-09", "--json"); err != nil {
		t.Fatal(err)
	}
	if sent["starts_at"] != "2026-10-08T00:00:00Z" || sent["ends_at"] != "2026-10-08T00:00:00Z" {
		t.Errorf("one day sent as %v .. %v, want ends_at 24h before --to", sent["starts_at"], sent["ends_at"])
	}
}

func TestReportCutsAWindowShorterThanADayByHour(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"metrics":[
			{"hour":"2026-10-09T20:00:00+00:00","spend":1000000},
			{"hour":"2026-10-09T21:00:00+00:00","spend":2000000},
			{"hour":"2026-10-09T22:00:00+00:00","spend":4000000}]}}`)
	}))
	defer srv.Close()

	out, err := runCLI(t, srv, "", "report", "--from", "2026-10-09T20:00:00Z", "--to", "2026-10-09T22:00:00Z",
		"--by", "hour", "--fields", "SPEND")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "2.00") || strings.Contains(out, "4.00") {
		t.Errorf("want the 20:00 and 21:00 rows only; got:\n%s", out)
	}

	if _, err := runCLI(t, srv, "", "report", "--from", "2026-10-09T20:00:00Z", "--to", "2026-10-09T22:00:00Z",
		"--fields", "SPEND"); err == nil {
		t.Error("a sub-day window without --by hour should be refused, not answered with the whole day")
	}
}
