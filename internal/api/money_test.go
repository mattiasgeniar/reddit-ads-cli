package api

import "testing"

func TestDollarsToMicros(t *testing.T) {
	cases := map[float64]int64{
		0.50:    500_000,
		1:       1_000_000,
		0:       0,
		0.01:    10_000,
		12.34:   12_340_000,
		1000.99: 1_000_990_000,
	}
	for dollars, want := range cases {
		if got := DollarsToMicros(dollars); got != want {
			t.Errorf("DollarsToMicros(%v) = %d, want %d", dollars, got, want)
		}
	}
}

func TestDollarsToMicrosRoundsWithoutDrift(t *testing.T) {
	// 0.1 in float64 is 0.1000000000000000055..., a naive int64(0.1*1e6) truncates
	// to 99999. Rounding must give 100000.
	if got := DollarsToMicros(0.1); got != 100_000 {
		t.Errorf("DollarsToMicros(0.1) = %d, want 100000", got)
	}
	if got := DollarsToMicros(0.29); got != 290_000 {
		t.Errorf("DollarsToMicros(0.29) = %d, want 290000", got)
	}
}

func TestMicrosToDollarsRoundTrip(t *testing.T) {
	for _, micros := range []int64{0, 10_000, 500_000, 1_000_000, 12_340_000} {
		if got := DollarsToMicros(MicrosToDollars(micros)); got != micros {
			t.Errorf("round trip of %d micros = %d", micros, got)
		}
	}
}

func TestFormatMicros(t *testing.T) {
	cases := map[int64]string{
		500_000:    "0.50",
		1_000_000:  "1.00",
		0:          "0.00",
		12_340_000: "12.34",
		10_000:     "0.01",
	}
	for micros, want := range cases {
		if got := FormatMicros(micros); got != want {
			t.Errorf("FormatMicros(%d) = %q, want %q", micros, got, want)
		}
	}
}

func TestCentsToDollars(t *testing.T) {
	// CONVERSION_*_TOTAL_VALUE is scaled by 100, not 1e6.
	if got := CentsToDollars(150); got != 1.50 {
		t.Errorf("CentsToDollars(150) = %v, want 1.50", got)
	}
}
