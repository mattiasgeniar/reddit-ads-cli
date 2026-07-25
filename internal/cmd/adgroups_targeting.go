package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mattiasgeniar/reddit-ads-cli/internal/api"
	"github.com/spf13/cobra"
)

func (a *App) adGroupsTargetingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "targeting",
		Short: "Show and edit ad group targeting (subreddit communities)",
	}
	cmd.AddCommand(
		a.targetingShowCmd(),
		a.targetingEditCmd("add-community", "Add communities to targeting",
			api.FieldCommunities, api.AddToTargetingList),
		a.targetingEditCmd("remove-community", "Remove communities from targeting",
			api.FieldCommunities, api.RemoveFromTargetingList),
		a.targetingEditCmd("exclude-community", "Add communities to the exclusion list",
			api.FieldExcludedCommunities, api.AddToTargetingList),
	)
	return cmd
}

func (a *App) targetingShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <ad-group-id>",
		Short: "Show an ad group's targeting",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := a.apiClient()
			if err != nil {
				return err
			}
			g, err := client.AdGroup(args[0])
			if err != nil {
				return err
			}
			if a.jsonOut {
				return printJSON(a.out, g.Targeting)
			}

			communities, _ := api.TargetingListValues(g.Targeting, api.FieldCommunities)
			excluded, _ := api.TargetingListValues(g.Targeting, api.FieldExcludedCommunities)

			t := newTable("FIELD", "VALUE")
			t.add("communities", orDash(strings.Join(communities, ", ")))
			t.add("excluded_communities", orDash(strings.Join(excluded, ", ")))
			t.add("other_targeting_fields", orDash(strings.Join(otherTargetingKeys(g.Targeting), ", ")))
			t.render(a.out)
			return nil
		},
	}
}

// targetingEditFn is the signature shared by the api add/remove helpers.
type targetingEditFn func(json.RawMessage, api.TargetingListField, ...string) (json.RawMessage, error)

func (a *App) targetingEditCmd(use, short string, field api.TargetingListField, edit targetingEditFn) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <ad-group-id> <community-id> [community-id...]",
		Short: short,
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			adGroupID, ids := args[0], args[1:]
			client, err := a.apiClient()
			if err != nil {
				return err
			}

			// Read-modify-write: targeting replaces wholesale, so fetch the current
			// object, change only this field, and send it back whole.
			g, err := client.AdGroup(adGroupID)
			if err != nil {
				return err
			}
			updated, err := edit(g.Targeting, field, ids...)
			if err != nil {
				return err
			}

			before, _ := api.TargetingListValues(g.Targeting, field)
			after, _ := api.TargetingListValues(updated, field)
			ok, err := a.confirm(fmt.Sprintf("Set ad group %s targeting.%s from [%s] to [%s].",
				adGroupID, field, strings.Join(before, ", "), strings.Join(after, ", ")))
			if err != nil || !ok {
				return err
			}

			if err := client.PatchAdGroup(adGroupID, map[string]any{"targeting": updated}); err != nil {
				return err
			}
			a.reportWrite("adgroup", adGroupID, fmt.Sprintf("targeting.%s now [%s]", field, strings.Join(after, ", ")))
			return nil
		},
	}
}

// otherTargetingKeys lists the non-empty targeting keys other than the two community
// lists, so `show` hints at what else is set without dumping every field.
func otherTargetingKeys(targeting json.RawMessage) []string {
	obj := map[string]json.RawMessage{}
	if len(targeting) == 0 || string(targeting) == "null" {
		return nil
	}
	if err := json.Unmarshal(targeting, &obj); err != nil {
		return nil
	}
	var keys []string
	for k, v := range obj {
		if k == string(api.FieldCommunities) || k == string(api.FieldExcludedCommunities) {
			continue
		}
		if len(v) == 0 || string(v) == "null" || string(v) == "[]" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
