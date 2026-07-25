package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strconv"
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
	var appID, appSecret, redirectURI, state, scopes string
	var port int
	var manual bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authorize the CLI (one-time, interactive)",
		Long: "Walks you through the one-time OAuth flow. Run it bare and it asks for anything " +
			"it needs, or pass --app-id/--app-secret/--redirect-uri for a non-interactive run " +
			"(CI). The App ID and Secret are the two values Reddit shows under Business > " +
			"Developer Portal > Developer Applications. It stores them and a permanent refresh " +
			"token, so you only do this once. When it prints the authorize URL, open it, click " +
			"Allow, and paste back either the code or the whole redirect URL from your address bar.",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := a.prompter()

			// Flags win over config; whatever is still missing gets prompted for.
			if appID != "" {
				a.cfg.AppID = appID
			}
			if a.cfg.AppID == "" {
				v, err := p.ask("App ID", "")
				if err != nil {
					return err
				}
				a.cfg.AppID = v
			}
			if a.cfg.AppID == "" {
				return fmt.Errorf("app id is required")
			}

			if appSecret != "" {
				a.cfg.AppSecret = appSecret
			}
			if a.cfg.AppSecret == "" {
				v, err := p.askSecret("App secret")
				if err != nil {
					return err
				}
				a.cfg.AppSecret = v
			}
			if a.cfg.AppSecret == "" {
				return fmt.Errorf("app secret is required")
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
			// --port rewrites the redirect URI's port. It must still match a redirect
			// registered on the app, so this is for when you registered a non-default one.
			if port != 0 {
				rewritten, err := withPort(a.cfg.RedirectURI, port)
				if err != nil {
					return err
				}
				a.cfg.RedirectURI = rewritten
			}

			// A random state per login is what actually makes the loopback callback
			// check meaningful: a fixed value is public and any local page could forge a
			// matching callback. --state overrides it only for scripted/testing use.
			if state == "" {
				s, err := randomState()
				if err != nil {
					return err
				}
				state = s
			}

			authURL := api.AuthorizeURL(a.cfg.AppID, a.cfg.RedirectURI, state, scopes)
			code, err := a.obtainAuthCode(p, authURL, a.cfg.RedirectURI, state, manual)
			if err != nil {
				return err
			}
			if code == "" {
				return fmt.Errorf("no authorization code captured")
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

			// Pick a default ad account now so `campaigns list` and friends work without
			// --account. Best-effort: auth already succeeded, so a listing hiccup here
			// just prints a hint instead of failing the login.
			if a.cfg.DefaultAdAccountID == "" {
				if err := a.setupDefaultAccount(p); err != nil {
					fmt.Fprintf(a.out, "Could not list your ad accounts (%v).\n", err)
					fmt.Fprintln(a.out, "Set one later with `default_ad_account_id` in config or pass --account.")
				}
			}

			fmt.Fprintln(a.out, "Try `reddit-ads auth status` or `reddit-ads accounts list`.")
			return nil
		},
	}
	cmd.Flags().StringVar(&appID, "app-id", "", "the App ID from Reddit's Developer Applications (prompted if unset; persisted)")
	cmd.Flags().StringVar(&appSecret, "app-secret", "", "the Secret from Reddit's Developer Applications (prompted if unset; persisted)")
	cmd.Flags().StringVar(&redirectURI, "redirect-uri", "", "redirect URI registered on the developer app (prompted if unset; persisted)")
	cmd.Flags().IntVar(&port, "port", 0, "override the redirect URI port for local capture (must match a registered redirect)")
	cmd.Flags().BoolVar(&manual, "manual", false, "skip local capture and paste the code by hand (for remote/headless boxes)")
	cmd.Flags().StringVar(&state, "state", "", "opaque state echoed back in the redirect (default: a random per-login nonce)")
	cmd.Flags().StringVar(&scopes, "scopes", api.V1Scopes, "comma-separated OAuth scopes")
	return cmd
}

// obtainAuthCode gets the OAuth code, preferring a local loopback server that catches
// the browser redirect automatically. When the server can't bind (port taken) or the
// browser is on another machine, it falls back to a pasted code. When the server does
// bind, it waits on the redirect and a pasted line at once, so both cases just work.
func (a *App) obtainAuthCode(p *prompter, authURL, redirectURI, state string, manual bool) (string, error) {
	fmt.Fprintln(a.out, "\nOpen this URL, sign in as a business admin, and click Allow:")
	fmt.Fprintln(a.out, "\n  "+authURL)

	var loop *loopbackServer
	if !manual {
		s, err := startLoopback(redirectURI, state)
		if err != nil {
			fmt.Fprintf(a.out, "\nCould not capture the redirect locally on %s (%v).\n", redirectURI, err)
			fmt.Fprintln(a.out, "Free that port, or pass --port <free-port> and register it on your app. Falling back to paste.")
		} else {
			loop = s
			defer loop.shutdown()
		}
	}

	if loop == nil {
		pasted, err := p.ask("\nYou land on your redirect URI with ?code=... Paste the code or the full URL", "")
		if err != nil {
			return "", err
		}
		return extractAuthCode(pasted), nil
	}

	fmt.Fprintf(a.out, "\nWaiting for the redirect on %s ...\n", redirectURI)
	fmt.Fprintln(a.out, "(Browser on another machine? Paste the code or the full redirect URL here instead.)")

	// Read a pasted line concurrently, so a remote browser (redirect can't reach us)
	// still works. An empty EOF read means no manual input; let the loopback win. If the
	// loopback wins on an interactive terminal this reader stays blocked in ReadString
	// until the process exits, which is fine here: the CLI does exactly one login per
	// process. (Cancelling a blocking stdin read portably is not worth the fragility.)
	pasteCh := make(chan string, 1)
	go func() {
		line, err := p.in.ReadString('\n')
		c := extractAuthCode(line)
		if c == "" && err != nil {
			return
		}
		pasteCh <- c
	}()

	select {
	case res := <-loop.result:
		if res.err != nil {
			return "", res.err
		}
		fmt.Fprintln(a.out, "Captured the redirect. Authentication successful.")
		return res.code, nil
	case c := <-pasteCh:
		return c, nil
	}
}

// setupDefaultAccount resolves the user's ad accounts right after login and records a
// default_ad_account_id: automatically when there is exactly one, or by asking which to
// use when there are several. It only returns an error for an actual listing failure;
// "no accounts" and "user skipped the prompt" are normal outcomes.
func (a *App) setupDefaultAccount(p *prompter) error {
	client, err := a.apiClient()
	if err != nil {
		return err
	}
	accounts, err := client.AdAccounts()
	if err != nil {
		return err
	}

	if len(accounts) == 0 {
		fmt.Fprintln(a.out, "No ad accounts are visible on this login yet.")
		return nil
	}
	if len(accounts) == 1 {
		return a.saveDefaultAccount(accounts[0])
	}

	fmt.Fprintln(a.out, "\nYou can access several ad accounts:")
	t := newTable("#", "ID", "NAME", "CURRENCY")
	for i, acct := range accounts {
		t.add(strconv.Itoa(i+1), acct.ID, orDash(acct.Name), orDash(acct.Currency))
	}
	t.render(a.out)

	choice, err := p.ask("Which number should be the default ad account? (enter to skip)", "")
	if err != nil || choice == "" {
		fmt.Fprintln(a.out, "No default set. Pass --account or set default_ad_account_id per command.")
		return nil
	}
	n, err := strconv.Atoi(choice)
	if err != nil || n < 1 || n > len(accounts) {
		fmt.Fprintf(a.out, "%q is not one of 1-%d; no default set.\n", choice, len(accounts))
		return nil
	}
	return a.saveDefaultAccount(accounts[n-1])
}

// saveDefaultAccount persists an ad account as the default and confirms it.
func (a *App) saveDefaultAccount(acct api.AdAccount) error {
	a.cfg.DefaultAdAccountID = acct.ID
	if err := a.cfg.Save(); err != nil {
		return err
	}
	label := acct.ID
	if acct.Name != "" {
		label = fmt.Sprintf("%s (%s)", acct.ID, acct.Name)
	}
	fmt.Fprintf(a.out, "Default ad account set to %s.\n", label)
	return nil
}

// randomState returns a random hex nonce for the OAuth state parameter.
func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// withPort replaces the port in a URL, leaving scheme, host, and path intact.
func withPort(rawURL string, port int) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	u.Host = net.JoinHostPort(u.Hostname(), strconv.Itoa(port))
	return u.String(), nil
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
