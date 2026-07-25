package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestExtractAuthCode(t *testing.T) {
	cases := map[string]string{
		"abc123":     "abc123",
		"abc123#_":   "abc123",
		"  abc123  ": "abc123",
		"https://cb.example/callback?code=abc123":         "abc123",
		"https://cb.example/callback?code=abc123#_":       "abc123",
		"https://cb.example/cb?state=x&code=abc123&foo=y": "abc123",
	}
	for input, want := range cases {
		if got := extractAuthCode(input); got != want {
			t.Errorf("extractAuthCode(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "", "x", "y"); got != "x" {
		t.Errorf("firstNonEmpty = %q, want x", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Errorf("firstNonEmpty of all-empty = %q, want empty", got)
	}
}

func TestPrompterAskUsesDefaultOnEmptyLine(t *testing.T) {
	app := &App{out: &bytes.Buffer{}, in: strings.NewReader("\n")}
	p := app.prompter()
	got, err := p.ask("Redirect URI", "http://localhost:8080/reddit-ads/callback")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://localhost:8080/reddit-ads/callback" {
		t.Errorf("empty answer should fall back to default, got %q", got)
	}
}

func TestPrompterAskTakesTypedValue(t *testing.T) {
	app := &App{out: &bytes.Buffer{}, in: strings.NewReader("https://custom/cb\n")}
	p := app.prompter()
	got, _ := p.ask("Redirect URI", "http://localhost:8080")
	if got != "https://custom/cb" {
		t.Errorf("got %q, want the typed value", got)
	}
}
