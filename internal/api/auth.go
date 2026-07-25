package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mattiasgeniar/reddit-ads-cli/internal/config"
)

// Reddit's OAuth endpoints live on www.reddit.com, not on the ads-api host.
const authorizeURL = "https://www.reddit.com/api/v1/authorize"

// accessTokenURL is a var, not a const, only so tests can point the token exchange at
// an httptest server. Production never changes it.
var accessTokenURL = "https://www.reddit.com/api/v1/access_token"

// V1Scopes are the scopes v1 needs. These are Reddit's real ads scopes, NOT the
// ads:read / ads:manage that third-party blog posts repeat.
const V1Scopes = "adsread,adsedit"

// AuthorizeURL builds the URL the user opens once to grant access. duration=permanent
// makes Reddit return a refresh token. state is echoed back in the redirect so the
// user can sanity-check it.
func AuthorizeURL(clientID, redirectURI, state, scopes string) string {
	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("response_type", "code")
	q.Set("state", state)
	q.Set("redirect_uri", redirectURI)
	q.Set("duration", "permanent")
	q.Set("scope", scopes)
	return authorizeURL + "?" + q.Encode()
}

// tokenResponse is the shape of both the code-exchange and refresh responses.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
}

// ExchangeCode swaps a one-time authorization code for a refresh token and an initial
// access token. The pasted code is single-use and valid for ten minutes.
func ExchangeCode(cfg *config.Config, code, redirectURI string) (refreshToken string, token *config.Token, err error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)

	tr, err := postToken(cfg, form)
	if err != nil {
		return "", nil, err
	}
	if tr.RefreshToken == "" {
		return "", nil, fmt.Errorf("no refresh token returned; was the authorize URL built with duration=permanent?")
	}
	return tr.RefreshToken, tokenFrom(tr), nil
}

// RefreshAccessToken uses the stored refresh token to mint a new access token.
func RefreshAccessToken(cfg *config.Config) (*config.Token, error) {
	if cfg.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token; run `reddit-ads auth login` first")
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", cfg.RefreshToken)

	tr, err := postToken(cfg, form)
	if err != nil {
		return nil, err
	}
	return tokenFrom(tr), nil
}

func tokenFrom(tr *tokenResponse) *config.Token {
	// Subtract a minute of slack so we never present a token that expires mid-flight.
	return &config.Token{
		AccessToken: tr.AccessToken,
		ExpiresAt:   time.Now().Add(time.Duration(tr.ExpiresIn)*time.Second - time.Minute),
	}
}

func postToken(cfg *config.Config, form url.Values) (*tokenResponse, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, fmt.Errorf("client_id and client_secret are required for the token exchange")
	}

	req, err := http.NewRequest(http.MethodPost, accessTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(cfg.ClientID, cfg.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", cfg.UserAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("decoding token response: %w", err)
	}
	if tr.Error != "" {
		return nil, fmt.Errorf("token endpoint error: %s", tr.Error)
	}
	if tr.AccessToken == "" {
		return nil, fmt.Errorf("token endpoint returned no access_token")
	}
	return &tr, nil
}
