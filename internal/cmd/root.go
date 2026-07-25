// Package cmd wires the reddit-ads CLI: global flags, a shared client, and the
// command groups (auth, accounts, campaigns, adgroups, ads, communities, report).
package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mattiasgeniar/reddit-ads-cli/internal/api"
	"github.com/mattiasgeniar/reddit-ads-cli/internal/config"
	"github.com/spf13/cobra"
)

// App holds shared state for one CLI invocation: the loaded config, the lazily-built
// client, the resolved global flags, and the IO streams.
type App struct {
	cfg    *config.Config
	client *api.Client

	jsonOut   bool
	account   string
	profile   string
	yes       bool
	dryRun    bool
	configDir string

	out io.Writer
	in  io.Reader
}

// Execute builds the command tree and runs it. It is the single entry point from main.
func Execute() error {
	app := &App{out: os.Stdout, in: os.Stdin}
	return app.rootCmd().Execute()
}

func (a *App) rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "reddit-ads",
		Short:         "Manage Reddit Ads campaigns from the terminal",
		Version:       config.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		// Load config for every command. The client is built lazily so `auth login`
		// works before any token exists.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(a.configDir)
			if err != nil {
				return err
			}
			a.cfg = cfg
			return nil
		},
	}

	pf := root.PersistentFlags()
	pf.BoolVar(&a.jsonOut, "json", false, "output raw JSON instead of a table")
	pf.StringVar(&a.account, "account", "", "ad account id (overrides default_ad_account_id)")
	pf.StringVar(&a.profile, "profile", "", "profile id (overrides default_profile_id)")
	pf.BoolVar(&a.yes, "yes", false, "skip the confirmation prompt on writes")
	pf.BoolVar(&a.dryRun, "dry-run", false, "print the request that would be sent, do not send it")
	pf.StringVar(&a.configDir, "config", "", "config directory (default ~/.config/reddit-ads)")

	root.AddCommand(
		a.authCmd(),
		a.accountsCmd(),
		a.campaignsCmd(),
		a.adGroupsCmd(),
		a.adsCmd(),
		a.communitiesCmd(),
		a.reportCmd(),
	)
	return root
}

// apiClient builds the client on first use, wiring in --dry-run and the output stream.
func (a *App) apiClient() (*api.Client, error) {
	if a.client != nil {
		return a.client, nil
	}
	c, err := api.New(a.cfg, a.out)
	if err != nil {
		return nil, err
	}
	c.DryRun = a.dryRun
	a.client = c
	return c, nil
}

// accountID resolves the ad account: --account flag, else default_ad_account_id.
func (a *App) accountID() (string, error) {
	if a.account != "" {
		return a.account, nil
	}
	if a.cfg.DefaultAdAccountID != "" {
		return a.cfg.DefaultAdAccountID, nil
	}
	return "", fmt.Errorf("no ad account: pass --account or set default_ad_account_id in config")
}

// profileID resolves the profile: --profile flag, else default_profile_id.
func (a *App) profileID() (string, error) {
	if a.profile != "" {
		return a.profile, nil
	}
	if a.cfg.DefaultProfileID != "" {
		return a.cfg.DefaultProfileID, nil
	}
	return "", fmt.Errorf("no profile: pass --profile or set default_profile_id in config")
}

// confirm returns true if the user confirms a write. --yes and --dry-run skip the
// prompt (dry-run because nothing is sent anyway).
func (a *App) confirm(action string) (bool, error) {
	if a.yes || a.dryRun {
		return true, nil
	}
	fmt.Fprintf(a.out, "%s Continue? [y/N] ", action)
	reader := bufio.NewReader(a.in)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
