package cmd

import (
	"fmt"

	"github.com/mattiasgeniar/reddit-ads-cli/internal/api"
	"github.com/spf13/cobra"
)

func (a *App) campaignsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "campaigns",
		Short: "List, inspect, and steer campaigns",
	}
	cmd.AddCommand(
		a.campaignsListCmd(),
		a.campaignsGetCmd(),
		a.statusWriteCmd("pause", "Pause a campaign", "PAUSED", a.patchCampaignStatus),
		a.statusWriteCmd("resume", "Resume a campaign", "ACTIVE", a.patchCampaignStatus),
		a.campaignsBudgetCmd(),
	)
	return cmd
}

func (a *App) campaignsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List campaigns in the ad account",
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
				raw, err := client.GetList(api.CampaignsPath(account))
				if err != nil {
					return err
				}
				return printJSONValue(a.out, raw)
			}
			campaigns, err := client.Campaigns(account)
			if err != nil {
				return err
			}
			t := newTable("ID", "NAME", "STATUS", "EFFECTIVE", "OBJECTIVE", "SPEND_CAP", "CBO")
			for _, c := range campaigns {
				t.add(c.ID, orDash(c.Name), c.ConfiguredStatus, orDash(c.EffectiveStatus),
					orDash(c.Objective), dollarsOrDash(c.SpendCap), yesNo(c.IsCBO))
			}
			t.render(a.out)
			return nil
		},
	}
}

func (a *App) campaignsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <campaign-id>",
		Short: "Show one campaign",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := a.apiClient()
			if err != nil {
				return err
			}
			if a.jsonOut {
				raw, err := client.GetRaw("/campaigns/" + args[0])
				if err != nil {
					return err
				}
				return printJSON(a.out, raw)
			}
			c, err := client.Campaign(args[0])
			if err != nil {
				return err
			}
			t := newTable("FIELD", "VALUE")
			t.add("id", c.ID)
			t.add("name", orDash(c.Name))
			t.add("configured_status", c.ConfiguredStatus)
			t.add("effective_status", orDash(c.EffectiveStatus))
			t.add("objective", orDash(c.Objective))
			t.add("spend_cap", dollarsOrDash(c.SpendCap))
			t.add("goal_value", dollarsOrDash(c.GoalValue))
			t.add("goal_type", orDash(c.GoalType))
			t.add("is_cbo", yesNo(c.IsCBO))
			t.add("conversion_pixel_id", orDash(c.ConversionPixelID))
			t.render(a.out)
			return nil
		},
	}
}

func (a *App) campaignsBudgetCmd() *cobra.Command {
	var spendCap, goal float64
	cmd := &cobra.Command{
		Use:   "budget <campaign-id>",
		Short: "Set a campaign's lifetime spend cap and/or CBO goal, in dollars",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data := map[string]any{}
			if cmd.Flags().Changed("spend-cap") {
				data["spend_cap"] = api.DollarsToMicros(spendCap)
			}
			if cmd.Flags().Changed("goal") {
				data["goal_value"] = api.DollarsToMicros(goal)
			}
			if len(data) == 0 {
				return fmt.Errorf("nothing to change: pass --spend-cap and/or --goal")
			}

			ok, err := a.confirm(fmt.Sprintf("Set budget on campaign %s to %s.", args[0], describeMicros(data)))
			if err != nil || !ok {
				return err
			}
			client, err := a.apiClient()
			if err != nil {
				return err
			}
			if err := client.PatchCampaign(args[0], data); err != nil {
				return err
			}
			a.reportWrite("campaign", args[0], "budget updated")
			return nil
		},
	}
	cmd.Flags().Float64Var(&spendCap, "spend-cap", 0, "lifetime spend cap in dollars")
	cmd.Flags().Float64Var(&goal, "goal", 0, "CBO goal value in dollars")
	return cmd
}

func (a *App) patchCampaignStatus(id, status string) error {
	client, err := a.apiClient()
	if err != nil {
		return err
	}
	return client.PatchCampaign(id, map[string]any{"configured_status": status})
}
