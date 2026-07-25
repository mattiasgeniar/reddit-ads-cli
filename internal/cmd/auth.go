package cmd

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/mattiasgeniar/reddit-ads-cli/internal/api"
	"github.com/spf13/cobra"
)

// defaultRedirectURI is offered as the default during `auth login`. Register this exact
// value on your developer app and you never have to think about it again. It never has
// to serve anything: the code is copied out of the address bar by hand.
const defaultRedirectURI = "http://localhost:8080/reddit-ads/callback"

func (a *App) authCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authorize the CLI and inspect the current token",
	}
	cmd.AddCommand(a.authLoginCmd(), a.authStatusCmd(), a.authRefreshCmd())
	return cmd
}

func (a *App) authLoginCmd() *cobra.Command {
	var clientID, clientSecret, redirectURI, state, scopes string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authorize the CLI (one-time, interactive)",
		Long: "Walks you through the one-time OAuth flow. Run it bare and it asks for anything " +
			"it needs, or pass --client-id/--client-secret/--redirect-uri for a non-interactive " +
			"run (CI). It stores your credentials and a permanent refresh token, so you only do " +
			"this once. When it prints the authorize URL, open it, click Allow, and paste back " +
			"either the code or the whole redirect URL from your address bar.",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := a.prompter()

			// Flags win over config; whatever is still missing gets prompted for.
			if clientID != "" {
				a.cfg.ClientID = clientID
			}
			if a.cfg.ClientID == "" {
				v, err := p.ask("Client ID", "")
				if err != nil {
					return err
				}
				a.cfg.ClientID = v
			}
			if a.cfg.ClientID == "" {
				return fmt.Errorf("client id is required")
			}

			if clientSecret != "" {
				a.cfg.ClientSecret = clientSecret
			}
			if a.cfg.ClientSecret == "" {
				v, err := p.askSecret("Client secret")
				if err != nil {
					return err
				}
				a.cfg.ClientSecret = v
			}
			if a.cfg.ClientSecret == "" {
				return fmt.Errorf("client secret is required")
			}

			if redirectURI != "" {
				a.cfg.RedirectURI = redirectURI
			}
			if a.cfg.RedirectURI == "" {
				v, err := p.ask("Redirect URI (must match your developer app)", defaultRedirectURI)
				if err != nil {
					return err
				}
				a.cfg.RedirectURI = v
			}

			authURL := api.AuthorizeURL(a.cfg.ClientID, a.cfg.RedirectURI, state, scopes)
			fmt.Fprintln(a.out, "\nOpen this URL, sign in as a business admin, and click Allow:")
			fmt.Fprintln(a.out, "\n  "+authURL)
			fmt.Fprintln(a.out, "\nYou land on your redirect URI with ?code=... in the address bar.")

			pasted, err := p.ask("\nPaste the code or the full redirect URL", "")
			if err != nil {
				return err
			}
			code := extractAuthCode(pasted)
			if code == "" {
				return fmt.Errorf("no code found in what you pasted")
			}

			refresh, token, err := api.ExchangeCode(a.cfg, code, a.cfg.RedirectURI)
			if err != nil {
				return err
			}
			a.cfg.RefreshToken = refresh
			if err := a.cfg.Save(); err != nil {
				return err
			}
			if err := a.cfg.SaveToken(token); err != nil {
				return err
			}

			fmt.Fprintf(a.out, "\nDone. Credentials and refresh token saved to %s (mode 600).\n", a.cfg.Dir())
			fmt.Fprintln(a.out, "Try `reddit-ads auth status` or `reddit-ads accounts list`.")
			return nil
		},
	}
	cmd.Flags().StringVar(&clientID, "client-id", "", "developer app client id (prompted if unset; persisted)")
	cmd.Flags().StringVar(&clientSecret, "client-secret", "", "developer app client secret (prompted if unset; persisted)")
	cmd.Flags().StringVar(&redirectURI, "redirect-uri", "", "redirect URI registered on the developer app (prompted if unset; persisted)")
	cmd.Flags().StringVar(&state, "state", "reddit-ads-cli", "opaque state value echoed back in the redirect")
	cmd.Flags().StringVar(&scopes, "scopes", api.V1Scopes, "comma-separated OAuth scopes")
	return cmd
}

// extractAuthCode pulls the OAuth code out of whatever the user pasted: a bare code, or
// the full redirect URL. Reddit appends a trailing #_ that has to come off either way.
func extractAuthCode(pasted string) string {
	pasted = strings.TrimSpace(pasted)
	pasted = strings.TrimSuffix(pasted, "#_")
	if !strings.Contains(pasted, "code=") {
		return pasted
	}
	if u, err := url.Parse(pasted); err == nil {
		if code := u.Query().Get("code"); code != "" {
			return strings.TrimSuffix(code, "#_")
		}
	}
	// Fall back to a manual scrape if it was not a parseable URL.
	rest := pasted[strings.Index(pasted, "code=")+len("code="):]
	rest, _, _ = strings.Cut(rest, "&")
	return strings.TrimSuffix(rest, "#_")
}

func (a *App) authStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show who the token belongs to and when it expires",
		RunE: func(cmd *cobra.Command, args []string) error {
			if a.cfg.RefreshToken == "" {
				return fmt.Errorf("not authorized: run `reddit-ads auth login`")
			}
			client, err := a.apiClient()
			if err != nil {
				return err
			}

			if a.jsonOut {
				raw, err := client.GetRaw("/me")
				if err != nil {
					return err
				}
				return printJSON(a.out, raw)
			}

			me, err := client.Me()
			if err != nil {
				return err
			}
			tok, err := a.cfg.LoadToken()
			if err != nil {
				return err
			}

			t := newTable("FIELD", "VALUE")
			t.add("reddit_username", orDash(me.RedditUsername))
			t.add("ads_user_id", orDash(me.ID))
			t.add("config_dir", a.cfg.Dir())
			if tok.AccessToken != "" {
				t.add("access_token_expires", tok.ExpiresAt.Local().Format("2006-01-02 15:04:05"))
			} else {
				t.add("access_token_expires", "no cached access token")
			}
			t.render(a.out)
			return nil
		},
	}
}

func (a *App) authRefreshCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "refresh",
		Short: "Force a new access token from the stored refresh token",
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := api.RefreshAccessToken(a.cfg)
			if err != nil {
				return err
			}
			if err := a.cfg.SaveToken(token); err != nil {
				return err
			}
			fmt.Fprintf(a.out, "Refreshed. New access token valid until %s\n",
				token.ExpiresAt.Local().Format("2006-01-02 15:04:05"))
			return nil
		},
	}
}
