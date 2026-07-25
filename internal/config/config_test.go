package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveEnforces600(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{ClientID: "abc", ClientSecret: "shh", dir: dir}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, configFileName))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("config.json mode = %o, want 600", perm)
	}
}

func TestSaveTokenEnforces600AndDoesNotWriteConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{dir: dir}
	if err := cfg.SaveToken(&Token{AccessToken: "tok", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, tokenFileName))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("token.json mode = %o, want 600", perm)
	}
	if _, err := os.Stat(filepath.Join(dir, configFileName)); !os.IsNotExist(err) {
		t.Error("SaveToken must not create config.json")
	}
}

func TestEnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	seed := &Config{ClientID: "from-file", RefreshToken: "file-refresh", dir: dir}
	if err := seed.Save(); err != nil {
		t.Fatal(err)
	}

	t.Setenv("REDDIT_ADS_CLIENT_ID", "from-env")
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientID != "from-env" {
		t.Errorf("ClientID = %q, want env value", cfg.ClientID)
	}
	if cfg.RefreshToken != "file-refresh" {
		t.Errorf("RefreshToken = %q, want file value (env unset)", cfg.RefreshToken)
	}
}

func TestLoadMissingFileIsNotAnError(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load of empty dir should not error: %v", err)
	}
	if cfg.UserAgent != DefaultUserAgent {
		t.Errorf("UserAgent = %q, want default", cfg.UserAgent)
	}
}

func TestTokenValid(t *testing.T) {
	skew := 30 * time.Second
	if (&Token{}).Valid(skew) {
		t.Error("empty token must be invalid")
	}
	fresh := &Token{AccessToken: "x", ExpiresAt: time.Now().Add(time.Hour)}
	if !fresh.Valid(skew) {
		t.Error("fresh token must be valid")
	}
	expiring := &Token{AccessToken: "x", ExpiresAt: time.Now().Add(10 * time.Second)}
	if expiring.Valid(skew) {
		t.Error("token within skew of expiry must be invalid")
	}
}
