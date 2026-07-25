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
