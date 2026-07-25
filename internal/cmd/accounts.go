package cmd

import "github.com/spf13/cobra"

func (a *App) accountsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "accounts",
		Short: "List ad accounts you can access",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List every ad account across your businesses",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := a.apiClient()
			if err != nil {
				return err
			}
			accounts, err := client.AdAccounts()
			if err != nil {
				return err
			}
			if a.jsonOut {
				return printJSONValue(a.out, accounts)
			}
			t := newTable("ID", "NAME", "CURRENCY", "TYPE", "TIMEZONE")
			for _, acct := range accounts {
				t.add(acct.ID, orDash(acct.Name), orDash(acct.Currency), orDash(acct.Type), orDash(acct.TimeZoneID))
			}
			t.render(a.out)
			return nil
		},
	})
	return cmd
}
