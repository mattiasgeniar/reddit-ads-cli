package cmd

import (
	"fmt"
	"strings"

	"github.com/mattiasgeniar/reddit-ads-cli/internal/api"
	"github.com/spf13/cobra"
)

func (a *App) adGroupsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "adgroups",
		Aliases: []string{"ad-groups"},
		Short:   "List, inspect, steer, and target ad groups",
	}
	cmd.AddCommand(
		a.adGroupsListCmd(),
		a.adGroupsGetCmd(),
		a.statusWriteCmd("pause", "Pause an ad group", "PAUSED", (*api.Client).PatchAdGroup),
		a.statusWriteCmd("resume", "Resume an ad group", "ACTIVE", (*api.Client).PatchAdGroup),
		a.adGroupsBidCmd(),
		a.adGroupsBudgetCmd(),
		a.adGroupsTargetingCmd(),
	)
	return cmd
}

func (a *App) adGroupsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List ad groups in the ad account",
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
				raw, err := client.GetList(api.AdGroupsPath(account))
				if err != nil {
					return err
				}
				return printJSONValue(a.out, raw)
			}
			groups, err := client.AdGroups(account)
			if err != nil {
				return err
			}
			t := newTable("ID", "NAME", "STATUS", "EFFECTIVE", "CAMPAIGN", "BID", "GOAL")
			for _, g := range groups {
				t.add(g.ID, orDash(g.Name), g.ConfiguredStatus, orDash(g.EffectiveStatus),
					orDash(g.CampaignID), dollarsOrDash(g.BidValue), dollarsOrDash(g.GoalValue))
			}
			t.render(a.out)
			return nil
		},
	}
}

func (a *App) adGroupsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <ad-group-id>",
		Short: "Show one ad group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := a.apiClient()
			if err != nil {
				return err
			}
			if a.jsonOut {
				raw, err := client.GetRaw("/ad_groups/" + args[0])
				if err != nil {
					return err
				}
				return printJSON(a.out, raw)
			}
			g, err := client.AdGroup(args[0])
			if err != nil {
				return err
			}
			t := newTable("FIELD", "VALUE")
			t.add("id", g.ID)
			t.add("name", orDash(g.Name))
			t.add("configured_status", g.ConfiguredStatus)
			t.add("effective_status", orDash(g.EffectiveStatus))
			t.add("campaign_id", orDash(g.CampaignID))
			t.add("bid_value", dollarsOrDash(g.BidValue))
			t.add("goal_type", orDash(g.GoalType))
			t.add("goal_value", dollarsOrDash(g.GoalValue))
			t.render(a.out)
			fmt.Fprintln(a.out, "\nuse `adgroups targeting show "+g.ID+"` for targeting")
			return nil
		},
	}
}

func (a *App) adGroupsBidCmd() *cobra.Command {
	var bid float64
	cmd := &cobra.Command{
		Use:   "bid <ad-group-id>",
		Short: "Set an ad group's bid, in dollars",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("bid") {
				return fmt.Errorf("pass --bid <dollars>")
			}
			micros, err := api.DollarsToMicros(bid)
			if err != nil {
				return fmt.Errorf("--bid: %w", err)
			}
			data := map[string]any{"bid_value": micros}
			ok, err := a.confirm(fmt.Sprintf("Set bid on ad group %s to %s.", args[0], describeMicros(data)))
			if err != nil || !ok {
				return err
			}
			client, err := a.apiClient()
			if err != nil {
				return err
			}
			if err := client.PatchAdGroup(args[0], data); err != nil {
				return err
			}
			a.reportWrite("adgroup", args[0], "bid updated")
			return nil
		},
	}
	cmd.Flags().Float64Var(&bid, "bid", 0, "bid value in dollars")
	return cmd
}

func (a *App) adGroupsBudgetCmd() *cobra.Command {
	var goal float64
	var goalType string
	cmd := &cobra.Command{
		Use:   "budget <ad-group-id>",
		Short: "Set an ad group's goal value (dollars) and optionally its goal type",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data := map[string]any{}
			if cmd.Flags().Changed("goal") {
				goalMicros, err := api.DollarsToMicros(goal)
				if err != nil {
					return fmt.Errorf("--goal: %w", err)
				}
				data["goal_value"] = goalMicros
			}
			if cmd.Flags().Changed("goal-type") {
				gt := strings.ToUpper(goalType)
				if gt != "DAILY_SPEND" && gt != "LIFETIME_SPEND" {
					return fmt.Errorf("goal-type must be DAILY_SPEND or LIFETIME_SPEND")
				}
				data["goal_type"] = gt
			}
			if len(data) == 0 {
				return fmt.Errorf("nothing to change: pass --goal and/or --goal-type")
			}
			ok, err := a.confirm(fmt.Sprintf("Set budget on ad group %s to %s.", args[0], describeMicros(data)))
			if err != nil || !ok {
				return err
			}
			client, err := a.apiClient()
			if err != nil {
				return err
			}
			if err := client.PatchAdGroup(args[0], data); err != nil {
				return err
			}
			a.reportWrite("adgroup", args[0], "budget updated")
			return nil
		},
	}
	cmd.Flags().Float64Var(&goal, "goal", 0, "goal value in dollars")
	cmd.Flags().StringVar(&goalType, "goal-type", "", "DAILY_SPEND or LIFETIME_SPEND")
	return cmd
}
