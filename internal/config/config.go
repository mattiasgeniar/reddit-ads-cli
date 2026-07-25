// Package config loads the CLI's static credentials and volatile token from two
// separate files, so a token refresh can never rewrite the credentials. Environment
// variables override the files, so cron and CI do not need a home directory.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Config holds the static credentials and defaults. It lives in config.json (mode
// 600) and is never rewritten by a token refresh.
type Config struct {
	ClientID           string `json:"client_id,omitempty"`
	ClientSecret       string `json:"client_secret,omitempty"`
	RefreshToken       string `json:"refresh_token,omitempty"`
	DefaultAdAccountID string `json:"default_ad_account_id,omitempty"`
	DefaultProfileID   string `json:"default_profile_id,omitempty"`
	UserAgent          string `json:"user_agent,omitempty"`

	// dir is the config directory these files were loaded from, so Save writes back
	// to the same place. Not serialized.
	dir string `json:"-"`
}

// Token is the cached OAuth access token and its expiry. It lives in token.json
// (mode 600), separate from Config so a refresh only ever touches this file.
type Token struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

const (
	configFileName = "config.json"
	tokenFileName  = "token.json"
)

// DefaultDir returns ~/.config/reddit-ads, honoring XDG_CONFIG_HOME.
func DefaultDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "reddit-ads"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "reddit-ads"), nil
}

// Load reads config.json from dir (empty dir means the default location), then
// overlays any environment variables. A missing file is not an error: the config
// may come entirely from the environment.
func Load(dir string) (*Config, error) {
	if dir == "" {
		d, err := DefaultDir()
		if err != nil {
			return nil, err
		}
		dir = d
	}

	cfg := &Config{dir: dir}
	data, err := os.ReadFile(filepath.Join(dir, configFileName))
	if err == nil {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", configFileName, err)
		}
		cfg.dir = dir
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	cfg.applyEnv()
	if cfg.UserAgent == "" {
		cfg.UserAgent = DefaultUserAgent
	}
	return cfg, nil
}

func (c *Config) applyEnv() {
	envOverride(&c.ClientID, "REDDIT_ADS_CLIENT_ID")
	envOverride(&c.ClientSecret, "REDDIT_ADS_CLIENT_SECRET")
	envOverride(&c.RefreshToken, "REDDIT_ADS_REFRESH_TOKEN")
	envOverride(&c.DefaultAdAccountID, "REDDIT_ADS_ACCOUNT_ID")
	envOverride(&c.DefaultProfileID, "REDDIT_ADS_PROFILE_ID")
	envOverride(&c.UserAgent, "REDDIT_ADS_USER_AGENT")
}

func envOverride(target *string, key string) {
	if v := os.Getenv(key); v != "" {
		*target = v
	}
}

// Dir returns the config directory in use.
func (c *Config) Dir() string { return c.dir }

// Save writes config.json with mode 600, creating the directory if needed. It does
// not touch token.json.
func (c *Config) Save() error {
	if c.dir == "" {
		d, err := DefaultDir()
		if err != nil {
			return err
		}
		c.dir = d
	}
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return err
	}
	// Persist the honest default rather than an empty user_agent.
	out := *c
	data, err := json.MarshalIndent(&out, "", "  ")
	if err != nil {
		return err
	}
	return writeFile600(filepath.Join(c.dir, configFileName), data)
}

// LoadToken reads the cached access token. A missing file returns a zero Token and
// no error: the caller treats an empty or expired token as "must refresh".
func (c *Config) LoadToken() (*Token, error) {
	tok := &Token{}
	data, err := os.ReadFile(filepath.Join(c.dir, tokenFileName))
	if errors.Is(err, os.ErrNotExist) {
		return tok, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, tok); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", tokenFileName, err)
	}
	return tok, nil
}

// SaveToken writes token.json with mode 600. It never touches config.json.
func (c *Config) SaveToken(tok *Token) error {
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	return writeFile600(filepath.Join(c.dir, tokenFileName), data)
}

// Valid reports whether the token is present and not within skew of expiry.
func (t *Token) Valid(skew time.Duration) bool {
	if t == nil || t.AccessToken == "" {
		return false
	}
	return time.Now().Add(skew).Before(t.ExpiresAt)
}

// writeFile600 writes atomically-ish with 600 perms. os.WriteFile respects the mode
// only on create, so we chmod afterwards to be certain even when the file existed.
func writeFile600(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}
