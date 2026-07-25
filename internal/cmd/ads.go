package cmd

import (
	"fmt"

	"github.com/mattiasgeniar/reddit-ads-cli/internal/api"
	"github.com/spf13/cobra"
)

func (a *App) adsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ads",
		Short: "List, inspect, pause/resume, and edit ad click URLs",
	}
	cmd.AddCommand(
		a.adsListCmd(),
		a.adsGetCmd(),
		a.statusWriteCmd("pause", "Pause an ad", "PAUSED", a.patchAdStatus),
		a.statusWriteCmd("resume", "Resume an ad", "ACTIVE", a.patchAdStatus),
		a.adsClickURLCmd(),
	)
	return cmd
}

func (a *App) adsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List ads in the ad account",
		RunE: func(cmd *cobra.Command, args []string) error {
			account, err := a.accountID()
			if err != nil {
				return err
			}
			client, err := a.apiClient()
			if err != nil {
				return err
			}
			if a.jsonOut {
				raw, err := client.GetList(api.AdsPath(account))
				if err != nil {
					return err
				}
				return printJSONValue(a.out, raw)
			}
			ads, err := client.Ads(account)
			if err != nil {
				return err
			}
			t := newTable("ID", "NAME", "STATUS", "EFFECTIVE", "AD_GROUP", "POST_ID")
			for _, ad := range ads {
				t.add(ad.ID, orDash(ad.Name), ad.ConfiguredStatus, orDash(ad.EffectiveStatus),
					orDash(ad.AdGroupID), orDash(ad.PostID))
			}
			t.render(a.out)
			return nil
		},
	}
}

func (a *App) adsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <ad-id>",
		Short: "Show one ad",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := a.apiClient()
			if err != nil {
				return err
			}
			if a.jsonOut {
				raw, err := client.GetRaw("/ads/" + args[0])
				if err != nil {
					return err
				}
				return printJSON(a.out, raw)
			}
			ad, err := client.Ad(args[0])
			if err != nil {
				return err
			}
			t := newTable("FIELD", "VALUE")
			t.add("id", ad.ID)
			t.add("name", orDash(ad.Name))
			t.add("configured_status", ad.ConfiguredStatus)
			t.add("effective_status", orDash(ad.EffectiveStatus))
			t.add("ad_group_id", orDash(ad.AdGroupID))
			t.add("post_id", orDash(ad.PostID))
			t.add("click_url", orDash(ad.ClickURL))
			t.render(a.out)
			return nil
		},
	}
}

func (a *App) adsClickURLCmd() *cobra.Command {
	var clickURL, queryParams string
	cmd := &cobra.Command{
		Use:   "click-url <ad-id>",
		Short: "Set an ad's click URL and/or query parameters (UTM changes, no new creative)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data := map[string]any{}
			if cmd.Flags().Changed("url") {
				data["click_url"] = clickURL
			}
			if cmd.Flags().Changed("query-params") {
				data["click_url_query_parameters"] = queryParams
			}
			if len(data) == 0 {
				return fmt.Errorf("nothing to change: pass --url and/or --query-params")
			}
			ok, err := a.confirm(fmt.Sprintf("Update click URL on ad %s.", args[0]))
			if err != nil || !ok {
				return err
			}
			client, err := a.apiClient()
			if err != nil {
				return err
			}
			if err := client.PatchAd(args[0], data); err != nil {
				return err
			}
			a.reportWrite("ad", args[0], "click url updated")
			return nil
		},
	}
	cmd.Flags().StringVar(&clickURL, "url", "", "new click_url")
	cmd.Flags().StringVar(&queryParams, "query-params", "", "new click_url_query_parameters (e.g. utm_source=reddit&utm_medium=cpc)")
	return cmd
}

func (a *App) patchAdStatus(id, status string) error {
	client, err := a.apiClient()
	if err != nil {
		return err
	}
	return client.PatchAd(id, map[string]any{"configured_status": status})
}
