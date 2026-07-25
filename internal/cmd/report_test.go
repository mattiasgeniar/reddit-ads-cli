package cmd

import (
	"encoding/json"
	"testing"
)

func TestNormalizeReportTime(t *testing.T) {
	got, err := normalizeReportTime("2026-07-01")
	if err != nil || got != "2026-07-01T00:00:00Z" {
		t.Fatalf("normalizeReportTime(date) = %q,%v", got, err)
	}
	got, err = normalizeReportTime("2026-07-01T09:00:00Z")
	if err != nil || got != "2026-07-01T09:00:00Z" {
		t.Fatalf("normalizeReportTime(timestamp) = %q,%v", got, err)
	}
	if _, err := normalizeReportTime("07/01/2026"); err == nil {
		t.Error("expected error on non-ISO date")
	}
}

func TestFormatReportCellConvertsMoney(t *testing.T) {
	// spend is microcurrency and must render as dollars.
	if got := formatReportCell("spend", json.RawMessage("500000")); got != "0.50" {
		t.Errorf("spend cell = %q, want 0.50", got)
	}
	if got := formatReportCell("cpc", json.RawMessage("1230000")); got != "1.23" {
		t.Errorf("cpc cell = %q, want 1.23", got)
	}
	// Averaged cost metrics arrive fractional in micros; they must still render as
	// dollars, not leak the raw microcurrency number.
	if got := formatReportCell("ecpm", json.RawMessage("1215559.16")); got != "1.22" {
		t.Errorf("fractional ecpm cell = %q, want 1.22", got)
	}
	// Cost metrics outside the default set (matched by pattern) still convert: a
	// cost_per_* field and an ECPA field (effective cost per action) are both micros.
	if got := formatReportCell("cost_per_conversion", json.RawMessage("500000")); got != "0.50" {
		t.Errorf("cost_per_conversion cell = %q, want 0.50", got)
	}
	if got := formatReportCell("conversion_purchase_ecpa", json.RawMessage("5000000")); got != "5.00" {
		t.Errorf("ecpa cell = %q, want 5.00", got)
	}
	// conversion total value is scaled by 100, not 1e6.
	if got := formatReportCell("conversion_signup_total_value", json.RawMessage("150")); got != "1.50" {
		t.Errorf("conversion value cell = %q, want 1.50", got)
	}
	// non-money integer passes through as its literal.
	if got := formatReportCell("clicks", json.RawMessage("42")); got != "42" {
		t.Errorf("clicks cell = %q, want 42", got)
	}
	// a quoted string breakdown key (community id) prints unquoted.
	if got := formatReportCell("community", json.RawMessage(`"t5_2qh1i"`)); got != "t5_2qh1i" {
		t.Errorf("community cell = %q, want t5_2qh1i", got)
	}
	if got := formatReportCell("spend", json.RawMessage("null")); got != "-" {
		t.Errorf("null cell = %q, want -", got)
	}
}

func TestIsMicroMetric(t *testing.T) {
	// Money: spend, cost_per_*, and the cp*/ecp* family, standalone or as a trailing
	// segment. These must convert 1e6 -> dollars.
	money := []string{
		"spend", "cpc", "cpv", "cpm", "ecpm", "cpa", "ecpa",
		"video_spend", "cost_per_conversion", "conversion_purchase_ecpa", "app_install_purchase_ecpa",
	}
	for _, k := range money {
		if !isMicroMetric(k) {
			t.Errorf("isMicroMetric(%q) = false, want true", k)
		}
	}
	// Not money: counts, rates, ids, and the cents-scaled conversion value fields (which
	// have their own branch and must not be treated as micros).
	notMoney := []string{
		"clicks", "impressions", "reach", "frequency", "ctr", "community", "campaign_id",
		"conversion_signup_total_value",
	}
	for _, k := range notMoney {
		if isMicroMetric(k) {
			t.Errorf("isMicroMetric(%q) = true, want false", k)
		}
	}
}

func TestResolveBreakdownsShorthand(t *testing.T) {
	got, err := resolveBreakdowns("", "community,date")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "COMMUNITY" || got[1] != "DATE" {
		t.Errorf("resolveBreakdowns(--by) = %v", got)
	}
	if _, err := resolveBreakdowns("", "nonsense"); err == nil {
		t.Error("expected error on unknown --by value")
	}
}
