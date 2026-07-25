package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mattiasgeniar/reddit-ads-cli/internal/api"
	"github.com/spf13/cobra"
)

// defaultReportFields are the metrics worth seeing on day one.
var defaultReportFields = []string{"IMPRESSIONS", "CLICKS", "CTR", "SPEND", "CPC"}

// byShorthand maps friendly --by values to the API breakdown enums, so a user can
// write `--by community` instead of `--breakdowns COMMUNITY`.
var byShorthand = map[string]string{
	"community": "COMMUNITY",
	"date":      "DATE",
	"hour":      "HOUR",
	"ad":        "AD_ID",
	"adgroup":   "AD_GROUP_ID",
	"campaign":  "CAMPAIGN_ID",
	"keyword":   "KEYWORD",
	"interest":  "INTEREST",
	"placement": "PLACEMENT",
	"country":   "COUNTRY",
	"gender":    "GENDER",
}

// microMetrics are the report metric keys (lowercase) scaled by 1e6; they render as
// dollars. The CONVERSION_*_TOTAL_VALUE keys are scaled by 100 instead, handled below.
var microMetrics = map[string]bool{
	"spend": true, "cpc": true, "cpv": true, "ecpm": true,
}

func (a *App) reportCmd() *cobra.Command {
	var from, to, fieldsFlag, breakdownsFlag, by, tz string
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Run a performance report (POST /ad_accounts/{id}/reports)",
		Long: "Dates are YYYY-MM-DD (expanded to hour granularity, --to is exclusive) or a full " +
			"YYYY-MM-DDTHH:00:00Z timestamp. Money fields print as dollars; use --json for raw " +
			"microcurrency exactly as Reddit returns it.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if from == "" || to == "" {
				return fmt.Errorf("--from and --to are required")
			}
			startsAt, err := normalizeReportTime(from)
			if err != nil {
				return fmt.Errorf("--from: %w", err)
			}
			endsAt, err := normalizeReportTime(to)
			if err != nil {
				return fmt.Errorf("--to: %w", err)
			}

			fields := splitUpper(fieldsFlag)
			if len(fields) == 0 {
				fields = defaultReportFields
			}
			breakdowns, err := resolveBreakdowns(breakdownsFlag, by)
			if err != nil {
				return err
			}

			account, err := a.accountID()
			if err != nil {
				return err
			}
			client, err := a.apiClient()
			if err != nil {
				return err
			}

			req := api.ReportRequest{
				StartsAt:   startsAt,
				EndsAt:     endsAt,
				Fields:     fields,
				Breakdowns: breakdowns,
				TimeZoneID: tz,
			}

			if a.jsonOut {
				raw, err := client.ReportRaw(account, req)
				if err != nil {
					return err
				}
				return printJSON(a.out, raw)
			}

			rows, err := client.Report(account, req)
			if err != nil {
				return err
			}
			a.renderReport(rows, breakdowns, fields)
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "start date YYYY-MM-DD (required)")
	cmd.Flags().StringVar(&to, "to", "", "end date YYYY-MM-DD, exclusive (required)")
	cmd.Flags().StringVar(&fieldsFlag, "fields", "", "comma-separated metric fields (default IMPRESSIONS,CLICKS,CTR,SPEND,CPC)")
	cmd.Flags().StringVar(&breakdownsFlag, "breakdowns", "", "comma-separated breakdown enums (max 3)")
	cmd.Flags().StringVar(&by, "by", "", "breakdown shorthand: community,date,hour,ad,adgroup,campaign,keyword,interest,placement,country,gender")
	cmd.Flags().StringVar(&tz, "tz", "", "time_zone_id for the report (defaults to the account timezone)")
	return cmd
}

func (a *App) renderReport(rows []map[string]json.RawMessage, breakdowns, fields []string) {
	// Columns: breakdown keys first (lowercased), then the metric fields.
	var cols []string
	cols = append(cols, lowerAll(breakdowns)...)
	cols = append(cols, lowerAll(fields)...)

	t := newTable(upperAll(cols)...)
	for _, row := range rows {
		cells := make([]string, 0, len(cols))
		for _, col := range cols {
			cells = append(cells, formatReportCell(col, row[col]))
		}
		t.add(cells...)
	}
	t.render(a.out)
	if len(rows) == 0 {
		fmt.Fprintln(a.out, "(no rows; metrics can take up to 6 hours to stabilize)")
	}
}

// formatReportCell renders one metric value, converting money keys to dollars.
func formatReportCell(key string, raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return "-"
	}
	if microMetrics[key] {
		var micros int64
		if err := json.Unmarshal(raw, &micros); err == nil {
			return api.FormatMicros(micros)
		}
	}
	if strings.HasPrefix(key, "conversion_") && strings.HasSuffix(key, "_total_value") {
		var cents int64
		if err := json.Unmarshal(raw, &cents); err == nil {
			return fmt.Sprintf("%.2f", api.CentsToDollars(cents))
		}
	}
	// Strings come quoted; numbers and everything else print as their JSON literal.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return strings.TrimSpace(string(raw))
}

// normalizeReportTime turns YYYY-MM-DD into YYYY-MM-DDTHH:00:00Z, or passes through a
// value that already looks like a full timestamp.
func normalizeReportTime(v string) (string, error) {
	v = strings.TrimSpace(v)
	if strings.Contains(v, "T") {
		return v, nil
	}
	if len(v) != 10 || strings.Count(v, "-") != 2 {
		return "", fmt.Errorf("expected YYYY-MM-DD, got %q", v)
	}
	return v + "T00:00:00Z", nil
}

func resolveBreakdowns(breakdownsFlag, by string) ([]string, error) {
	if by != "" {
		var out []string
		for _, b := range strings.Split(by, ",") {
			b = strings.ToLower(strings.TrimSpace(b))
			enum, ok := byShorthand[b]
			if !ok {
				return nil, fmt.Errorf("unknown --by value %q", b)
			}
			out = append(out, enum)
		}
		return out, nil
	}
	return splitUpper(breakdownsFlag), nil
}

func splitUpper(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.ToUpper(strings.TrimSpace(part)); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func lowerAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = strings.ToLower(s)
	}
	return out
}

func upperAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = strings.ToUpper(s)
	}
	return out
}
