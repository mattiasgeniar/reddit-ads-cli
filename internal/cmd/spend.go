package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// SpendRow is one advertisement on one day, in the shape shared by every CLI in
// this family so that whatever consumes it never has to know which platform a
// row came from.
//
// Deliberately the same field names as apple-ads-cli. A consumer keyed on
// `platform` can then hold every network's cost in one table.
type SpendRow struct {
	Platform     string `json:"platform"`
	Date         string `json:"date"`
	CampaignID   string `json:"campaign_id"`
	CampaignName string `json:"campaign_name,omitempty"`
	AdGroupID    string `json:"ad_group_id,omitempty"`
	AdID         string `json:"ad_id"`
	AdName       string `json:"ad_name,omitempty"`
	SpendCents   int64  `json:"spend_cents"`
	Currency     string `json:"currency"`
	Impressions  int64  `json:"impressions"`
	Clicks       int64  `json:"clicks"`
	Conversions  int64  `json:"conversions"`
}

// spendFields are the metrics --format=spend needs, regardless of what --fields
// says. Asking for them explicitly means the output shape does not depend on a
// flag somebody forgot.
//
// Reddit counts conversions as _CLICKS and _VIEWS, being click-attributed and
// view-attributed, and has no _COUNT or _TOTAL_ITEMS for every event type.
// TOTAL_ITEMS exists only for cart-shaped events and counts *items*, not
// conversions, so it is the wrong column even where it exists.
//
// Watch the spelling: Reddit writes SIGN_UP in the count fields and SIGNUP in
// the value fields. That is Reddit's inconsistency, not a typo here, and asking
// for a name it does not have fails the whole request with a 400 that lists
// every valid enum.
var spendFields = []string{
	"IMPRESSIONS",
	"CLICKS",
	"SPEND",
	"CONVERSION_SIGN_UP_CLICKS",
	"CONVERSION_PURCHASE_CLICKS",
}

// spendRows converts Reddit's report rows into the shared shape.
//
// Reddit reports money in microcurrency (1e6 per unit) and this family stores
// integer cents, so every value is divided by 10,000 exactly once, here.
// Rounding rather than truncating: truncation loses up to a cent per row in one
// direction only, which across a month of daily rows per ad is a visible bias in
// cost per subscriber.
func spendRows(rows []map[string]json.RawMessage, currency string) ([]SpendRow, error) {
	out := []SpendRow{}

	for _, row := range rows {
		spend, err := microsToCents(row["spend"])
		if err != nil {
			return nil, err
		}

		impressions := intField(row["impressions"])
		clicks := intField(row["clicks"])

		// A day an ad did not run comes back as a zero row. Storing it puts
		// rows with nothing in them into a cost table and invites a divide by
		// zero downstream.
		if spend == 0 && impressions == 0 && clicks == 0 {
			continue
		}

		out = append(out, SpendRow{
			Platform:     "reddit",
			Date:         dateField(row["date"]),
			CampaignID:   stringField(row["campaign_id"]),
			CampaignName: stringField(row["campaign_name"]),
			AdGroupID:    stringField(row["ad_group_id"]),

			// Empty string rather than absent when the report was not broken
			// down by ad: the consumer's unique index cannot compare two nulls
			// as equal, so a campaign-grain row would duplicate on re-import.
			AdID:        stringField(row["ad_id"]),
			AdName:      stringField(row["ad_name"]),
			SpendCents:  spend,
			Currency:    strings.ToUpper(currency),
			Impressions: impressions,
			Clicks:      clicks,
			Conversions: conversionsField(row),
		})
	}

	return out, nil
}

// microsToCents divides Reddit's microcurrency by 10,000 without going through a
// float any wider than the division itself needs.
func microsToCents(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 {
		return 0, nil
	}

	var micros json.Number
	if err := json.Unmarshal(raw, &micros); err != nil {
		// Reddit has been seen to send money as a quoted string. Falling back
		// rather than failing, because a report that errors on one odd row is
		// worse than one that reads it.
		var asString string
		if json.Unmarshal(raw, &asString) != nil {
			return 0, fmt.Errorf("spend %s is neither a number nor a string", raw)
		}
		micros = json.Number(asString)
	}

	f, err := micros.Float64()
	if err != nil {
		return 0, fmt.Errorf("parse spend %q: %w", micros, err)
	}

	return int64(roundHalfAwayFromZero(f / 10_000)), nil
}

func roundHalfAwayFromZero(f float64) float64 {
	if f < 0 {
		return -roundHalfAwayFromZero(-f)
	}

	return float64(int64(f + 0.5))
}

func intField(raw json.RawMessage) int64 {
	if len(raw) == 0 {
		return 0
	}

	var n json.Number
	if json.Unmarshal(raw, &n) != nil {
		return 0
	}

	i, err := n.Int64()
	if err != nil {
		f, ferr := n.Float64()
		if ferr != nil {
			return 0
		}

		return int64(f)
	}

	return i
}

// conversionsField sums whichever conversion columns the report carried.
//
// Which ones those are depends on the campaign's optimisation goal, and a report
// asked for one goal's metric returns nothing for another, so summing beats
// naming. Both attribution halves are counted: Reddit splits a conversion into
// click-attributed and view-attributed and neither is the whole number.
//
// Value and rate columns are deliberately excluded. _TOTAL_VALUE is money,
// _AVG_VALUE is money, _ECPA is money and _CVR is a ratio; adding any of them to
// a count produces a number that looks plausible and means nothing.
//
// This figure is only ever what the *platform* believes. It lands in
// ad_spend.platform_conversions and must never be added to conversions counted
// from our own database.
func conversionsField(row map[string]json.RawMessage) int64 {
	var total int64

	for key, raw := range row {
		if !strings.HasPrefix(key, "conversion_") {
			continue
		}

		if strings.HasSuffix(key, "_clicks") || strings.HasSuffix(key, "_views") {
			total += intField(raw)
		}
	}

	return total
}

// stringField reads a value that may arrive as a JSON string or a JSON number.
// Reddit sends ids both ways depending on the breakdown, and an id must survive
// as text: a 64-bit id through float64 loses precision silently.
func stringField(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}

	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String()
	}

	return ""
}

// dateField trims Reddit's timestamps back to a calendar day.
//
// A report broken down by DATE returns "2026-09-17", one broken down by HOUR
// returns a full timestamp, and the consuming side stores a date either way.
func dateField(raw json.RawMessage) string {
	value := stringField(raw)
	if len(value) >= 10 {
		if _, err := strconv.Atoi(strings.ReplaceAll(value[:10], "-", "")); err == nil {
			return value[:10]
		}
	}

	return value
}
