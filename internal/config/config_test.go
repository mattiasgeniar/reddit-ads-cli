package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveEnforces600(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{AppID: "abc", AppSecret: "shh", dir: dir}
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

func TestLoadHardensLoosePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)
	// Simulate a config that was created or hand-edited world-readable.
	if err := os.WriteFile(path, []byte(`{"app_id":"x","app_secret":"secret"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("Load did not tighten loose config perms: got %o, want 600", perm)
	}
}

func TestLoadTokenHardensLoosePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, tokenFileName)
	if err := os.WriteFile(path, []byte(`{"access_token":"AT"}`), 0o640); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{dir: dir}
	if _, err := cfg.LoadToken(); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("LoadToken did not tighten loose token perms: got %o, want 600", perm)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	seed := &Config{AppID: "from-file", RefreshToken: "file-refresh", dir: dir}
	if err := seed.Save(); err != nil {
		t.Fatal(err)
	}

	t.Setenv("REDDIT_ADS_APP_ID", "from-env")
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AppID != "from-env" {
		t.Errorf("AppID = %q, want env value", cfg.AppID)
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
