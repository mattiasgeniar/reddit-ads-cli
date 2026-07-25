package cmd

import (
	"strconv"

	"github.com/mattiasgeniar/reddit-ads-cli/internal/api"
	"github.com/spf13/cobra"
)

func (a *App) communitiesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "communities",
		Aliases: []string{"community"},
		Short:   "Resolve subreddit names to targeting ids",
	}
	cmd.AddCommand(a.communitiesSearchCmd(), a.communitiesSuggestCmd())
	return cmd
}

func (a *App) communitiesSearchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search communities by name to get their targeting id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := a.apiClient()
			if err != nil {
				return err
			}
			if a.jsonOut {
				raw, err := client.SearchCommunitiesRaw(args[0])
				if err != nil {
					return err
				}
				return printJSONValue(a.out, raw)
			}
			results, err := client.SearchCommunities(args[0])
			if err != nil {
				return err
			}
			return a.renderCommunities(results)
		},
	}
}

func (a *App) communitiesSuggestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "suggest <community-name> [community-name...]",
		Short: "Suggest similar communities from one or more seed names",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := a.apiClient()
			if err != nil {
				return err
			}
			if a.jsonOut {
				raw, err := client.SuggestCommunitiesRaw(args)
				if err != nil {
					return err
				}
				return printJSONValue(a.out, raw)
			}
			results, err := client.SuggestCommunities(args)
			if err != nil {
				return err
			}
			return a.renderCommunities(results)
		},
	}
}

func (a *App) renderCommunities(results []api.Community) error {
	t := newTable("ID", "NAME", "SUBSCRIBERS")
	for _, c := range results {
		t.add(c.ID, orDash(c.Name), strconv.FormatInt(c.SubscriberCount, 10))
	}
	t.render(a.out)
	return nil
}
