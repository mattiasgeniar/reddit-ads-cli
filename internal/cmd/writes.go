package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mattiasgeniar/reddit-ads-cli/internal/api"
	"github.com/spf13/cobra"
)

// statusWriteCmd builds a pause/resume-style command: it takes one object id,
// confirms, and calls patch with configured_status set to the target. patch is a
// PatchCampaign/PatchAdGroup/PatchAd method expression, so pause/resume stay identical
// across campaigns, ad groups, and ads without a per-type wrapper.
func (a *App) statusWriteCmd(use, short, status string, patch func(*api.Client, string, map[string]any) error) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ok, err := a.confirm(fmt.Sprintf("Set %s to configured_status=%s.", args[0], status))
			if err != nil || !ok {
				return err
			}
			client, err := a.apiClient()
			if err != nil {
				return err
			}
			if err := patch(client, args[0], map[string]any{"configured_status": status}); err != nil {
				return err
			}
			a.reportWrite(strings.Fields(short)[0], args[0], "configured_status="+status)
			return nil
		},
	}
}

// reportWrite prints a one-line confirmation after a successful write. Under
// --dry-run the client already printed the request, so this stays quiet.
func (a *App) reportWrite(kind, id, change string) {
	if a.dryRun {
		return
	}
	fmt.Fprintf(a.out, "OK: %s %s (%s)\n", kind, id, change)
}

// describeMicros renders the money fields in a patch body as dollars for the
// confirmation prompt, so the user sees "spend_cap=$50.00" not the raw microcurrency.
func describeMicros(data map[string]any) string {
	parts := make([]string, 0, len(data))
	for k, v := range data {
		if micros, ok := v.(int64); ok {
			parts = append(parts, fmt.Sprintf("%s=$%s", k, api.FormatMicros(micros)))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
