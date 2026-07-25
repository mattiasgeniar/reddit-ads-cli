package cmd

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/mattiasgeniar/reddit-ads-cli/internal/api"
	"github.com/spf13/cobra"
)

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
		Short: "Run the one-time OAuth flow and store the refresh token",
		Long: "Prints the Reddit authorize URL. Open it, click Allow, then copy the `code` " +
			"query parameter out of the redirect URL and paste it here. The code is valid " +
			"for ten minutes and is single-use.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Flags override config so first-time setup can supply credentials inline.
			if clientID != "" {
				a.cfg.ClientID = clientID
			}
			if clientSecret != "" {
				a.cfg.ClientSecret = clientSecret
			}
			if a.cfg.ClientID == "" || a.cfg.ClientSecret == "" {
				return fmt.Errorf("client id and secret required: pass --client-id/--client-secret or set them in config")
			}
			if redirectURI == "" {
				return fmt.Errorf("--redirect-uri is required and must match the value registered on the developer app")
			}

			url := api.AuthorizeURL(a.cfg.ClientID, redirectURI, state, scopes)
			fmt.Fprintln(a.out, "1. Open this URL, sign in as a business admin, and click Allow:")
			fmt.Fprintln(a.out)
			fmt.Fprintln(a.out, url)
			fmt.Fprintln(a.out)
			fmt.Fprintln(a.out, "2. You will be redirected to your redirect URI with ?code=... in the address bar.")
			fmt.Fprint(a.out, "3. Paste the code value here (strip any trailing #_): ")

			reader := bufio.NewReader(a.in)
			line, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("reading code: %w", err)
			}
			code := strings.TrimSpace(line)
			code = strings.TrimSuffix(code, "#_")
			if code == "" {
				return fmt.Errorf("no code entered")
			}

			refresh, token, err := api.ExchangeCode(a.cfg, code, redirectURI)
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

			fmt.Fprintf(a.out, "\nAuthorized. Refresh token and credentials saved to %s\n", a.cfg.Dir())
			return nil
		},
	}
	cmd.Flags().StringVar(&clientID, "client-id", "", "developer app client id (persisted to config)")
	cmd.Flags().StringVar(&clientSecret, "client-secret", "", "developer app client secret (persisted to config)")
	cmd.Flags().StringVar(&redirectURI, "redirect-uri", "", "redirect URI registered on the developer app (required)")
	cmd.Flags().StringVar(&state, "state", "reddit-ads-cli", "opaque state value echoed back in the redirect")
	cmd.Flags().StringVar(&scopes, "scopes", api.V1Scopes, "comma-separated OAuth scopes")
	return cmd
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
