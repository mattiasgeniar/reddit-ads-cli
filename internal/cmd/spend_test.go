package cmd

import (
	"encoding/json"
	"testing"
)

func rawRow(t *testing.T, literal string) map[string]json.RawMessage {
	t.Helper()

	var row map[string]json.RawMessage
	if err := json.Unmarshal([]byte(literal), &row); err != nil {
		t.Fatalf("fixture is not valid json: %v", err)
	}

	return row
}

func TestMicrosToCentsDividesByTenThousand(t *testing.T) {
	cases := map[string]int64{
		`12550000`:   1255,
		`0`:          0,
		`70000`:      7,
		`1000000`:    100,
		`"12550000"`: 1255, // Reddit has been seen to quote money
		`-320000`:    -32,
	}

	for literal, want := range cases {
		t.Run(literal, func(t *testing.T) {
			got, err := microsToCents(json.RawMessage(literal))
			if err != nil {
				t.Fatalf("microsToCents: %v", err)
			}
			if got != want {
				t.Errorf("microsToCents(%s) = %d, want %d", literal, got, want)
			}
		})
	}
}

// Truncating instead of rounding loses up to a cent per row in one direction,
// which across a month of daily rows per ad is a visible bias in cost per
// subscriber rather than noise.
func TestMicrosToCentsRoundsRatherThanTruncates(t *testing.T) {
	cases := map[string]int64{
		`15000`: 2, // 1.5 cents rounds up
		`14999`: 1,
		`25000`: 3, // half away from zero, not to even
	}

	for literal, want := range cases {
		got, err := microsToCents(json.RawMessage(literal))
		if err != nil {
			t.Fatalf("microsToCents: %v", err)
		}
		if got != want {
			t.Errorf("microsToCents(%s) = %d, want %d", literal, got, want)
		}
	}
}

func TestMicrosToCentsRefusesSomethingThatIsNotANumber(t *testing.T) {
	if _, err := microsToCents(json.RawMessage(`"twelve euros"`)); err == nil {
		t.Fatal("want an error")
	}
}

func TestSpendRowsNormalisesAReportRow(t *testing.T) {
	rows := []map[string]json.RawMessage{
		rawRow(t, `{"ad_id":"t2_ad1","campaign_id":"t2_cmp1","ad_group_id":"t2_ag1",
                    "date":"2026-09-17","spend":12550000,"impressions":5000,"clicks":120,
                    "conversion_sign_up_clicks":5,"conversion_sign_up_views":2}`),
	}

	out, err := spendRows(rows, "eur")
	if err != nil {
		t.Fatalf("spendRows: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d rows, want 1", len(out))
	}

	row := out[0]
	if row.Platform != "reddit" {
		t.Errorf("platform = %q", row.Platform)
	}
	if row.SpendCents != 1255 {
		t.Errorf("spend_cents = %d, want 1255", row.SpendCents)
	}
	if row.Currency != "EUR" {
		t.Errorf("currency = %q, want it upper-cased", row.Currency)
	}
	if row.AdID != "t2_ad1" || row.CampaignID != "t2_cmp1" || row.AdGroupID != "t2_ag1" {
		t.Errorf("ids did not survive: %+v", row)
	}
	if row.Conversions != 7 {
		t.Errorf("conversions = %d, want 7", row.Conversions)
	}
}

// Reddit returns every day in the range whether or not the ad ran.
func TestSpendRowsDropsDaysWhereNothingHappened(t *testing.T) {
	rows := []map[string]json.RawMessage{
		rawRow(t, `{"ad_id":"a","date":"2026-09-16","spend":0,"impressions":0,"clicks":0}`),
		rawRow(t, `{"ad_id":"a","date":"2026-09-17","spend":10000,"impressions":5,"clicks":1}`),
	}

	out, err := spendRows(rows, "EUR")
	if err != nil {
		t.Fatalf("spendRows: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d rows, want 1", len(out))
	}
	if out[0].Date != "2026-09-17" {
		t.Errorf("kept the wrong day: %q", out[0].Date)
	}
}

// An impression with no spend yet is real and must survive; only a row with
// nothing at all in it gets dropped.
func TestSpendRowsKeepsARowWithImpressionsButNoSpend(t *testing.T) {
	rows := []map[string]json.RawMessage{
		rawRow(t, `{"ad_id":"a","date":"2026-09-17","spend":0,"impressions":40,"clicks":0}`),
	}

	out, err := spendRows(rows, "EUR")
	if err != nil {
		t.Fatalf("spendRows: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d rows, want 1", len(out))
	}
}

// Which conversion metric a report carries depends on the campaign's
// optimisation goal, so they are summed rather than read by name. Both halves
// of Reddit's attribution count: click-attributed and view-attributed.
func TestSpendRowsSumsWhicheverConversionMetricTheReportCarried(t *testing.T) {
	rows := []map[string]json.RawMessage{
		rawRow(t, `{"ad_id":"a","date":"2026-09-17","spend":10000,"clicks":1,
                    "conversion_sign_up_clicks":3,"conversion_sign_up_views":1,
                    "conversion_purchase_clicks":2}`),
	}

	out, err := spendRows(rows, "EUR")
	if err != nil {
		t.Fatalf("spendRows: %v", err)
	}
	if out[0].Conversions != 6 {
		t.Errorf("conversions = %d, want 3+1+2=6", out[0].Conversions)
	}
}

// Money and rate columns share the conversion_ prefix and adding any of them to
// a count gives a number that looks plausible and means nothing. Reddit's own
// enum list is the source of these names, including its inconsistent spelling:
// SIGN_UP in the count fields, SIGNUP in the value fields.
func TestSpendRowsNeverAddsConversionValueOrRateToTheCount(t *testing.T) {
	rows := []map[string]json.RawMessage{
		rawRow(t, `{"ad_id":"a","date":"2026-09-17","spend":10000,"clicks":1,
                    "conversion_sign_up_clicks":4,
                    "conversion_signup_total_value":999900,
                    "conversion_signup_avg_value":24999,
                    "conversion_sign_up_ecpa":50000,
                    "conversion_roas":312}`),
	}

	out, err := spendRows(rows, "EUR")
	if err != nil {
		t.Fatalf("spendRows: %v", err)
	}
	if out[0].Conversions != 4 {
		t.Errorf("conversions = %d, want 4: only _clicks and _views are counts", out[0].Conversions)
	}
}

// The field names --format=spend asks for must be ones Reddit actually has. A
// name it does not recognise fails the entire request with a 400, which is how
// CONVERSION_SIGNUP_TOTAL_ITEMS (a name that does not exist) was caught.
func TestSpendFieldsUseRedditsRealEnumNames(t *testing.T) {
	valid := map[string]bool{
		"IMPRESSIONS":                true,
		"CLICKS":                     true,
		"SPEND":                      true,
		"CONVERSION_SIGN_UP_CLICKS":  true,
		"CONVERSION_SIGN_UP_VIEWS":   true,
		"CONVERSION_PURCHASE_CLICKS": true,
		"CONVERSION_PURCHASE_VIEWS":  true,
	}

	for _, field := range spendFields {
		if !valid[field] {
			t.Errorf("%q is not a Reddit report enum", field)
		}
	}
}

// Ids arrive as JSON numbers on some breakdowns. A 64-bit id through float64
// loses precision silently, so they must be read as text.
func TestSpendRowsKeepsNumericIdsExact(t *testing.T) {
	rows := []map[string]json.RawMessage{
		rawRow(t, `{"ad_id":9007199254740993,"campaign_id":9007199254740995,
                    "date":"2026-09-17","spend":10000,"clicks":1}`),
	}

	out, err := spendRows(rows, "EUR")
	if err != nil {
		t.Fatalf("spendRows: %v", err)
	}
	if out[0].AdID != "9007199254740993" {
		t.Errorf("ad id = %q, lost precision", out[0].AdID)
	}
	if out[0].CampaignID != "9007199254740995" {
		t.Errorf("campaign id = %q, lost precision", out[0].CampaignID)
	}
}

// A report broken down by HOUR returns a full timestamp where one broken down
// by DATE returns a day, and the consuming side stores a date either way.
func TestSpendRowsTrimsAnHourlyTimestampToItsDay(t *testing.T) {
	rows := []map[string]json.RawMessage{
		rawRow(t, `{"ad_id":"a","date":"2026-09-17T13:00:00Z","spend":10000,"clicks":1}`),
	}

	out, err := spendRows(rows, "EUR")
	if err != nil {
		t.Fatalf("spendRows: %v", err)
	}
	if out[0].Date != "2026-09-17" {
		t.Errorf("date = %q, want it trimmed to the day", out[0].Date)
	}
}

// A campaign-grain report has no ad id, and the consumer's unique index cannot
// compare two nulls as equal, so the field must be an empty string and present.
func TestSpendRowsEmitsAnEmptyAdIdRatherThanOmittingIt(t *testing.T) {
	rows := []map[string]json.RawMessage{
		rawRow(t, `{"campaign_id":"t2_cmp1","date":"2026-09-17","spend":10000,"clicks":1}`),
	}

	out, err := spendRows(rows, "EUR")
	if err != nil {
		t.Fatalf("spendRows: %v", err)
	}

	encoded, err := json.Marshal(out[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := decoded["ad_id"]; !ok {
		t.Error("ad_id must be present even when empty")
	}
}

func TestSpendRowsIsEmptyRatherThanNull(t *testing.T) {
	out, err := spendRows(nil, "EUR")
	if err != nil {
		t.Fatalf("spendRows: %v", err)
	}

	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(encoded) != "[]" {
		t.Errorf("marshalled to %s, want []", encoded)
	}
}
