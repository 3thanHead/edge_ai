package saturation

import (
	"testing"

	"github.com/3thanHead/iot_ai/niche-finder/internal/etsy"
)

func TestMeterFromCount(t *testing.T) {
	cases := []struct{ count, lo, hi int }{
		{0, 0, 0},
		{10, 15, 30},
		{1000, 55, 65},
		{100_000, 100, 100},
		{500_000, 100, 100},
	}
	for _, c := range cases {
		if got := meterFromCount(c.count); got < c.lo || got > c.hi {
			t.Errorf("meterFromCount(%d) = %d, want [%d,%d]", c.count, got, c.lo, c.hi)
		}
	}
}

func TestMeterMonotonic(t *testing.T) {
	prev := -1
	for _, n := range []int{0, 5, 50, 500, 5000, 50000, 100000} {
		v := meterFromCount(n)
		if v < prev {
			t.Errorf("not monotonic at %d: %d < %d", n, v, prev)
		}
		prev = v
	}
}

func TestParseEbayCount(t *testing.T) {
	html := `<h1 class="srp-controls__count-heading"><span class="BOLD">12,345</span> results for vintage map</h1>`
	got, err := parseEbayCount(html)
	if err != nil || got != 12345 {
		t.Fatalf("parseEbayCount = %d, %v; want 12345", got, err)
	}
}

func TestParseEbayCountMissing(t *testing.T) {
	if _, err := parseEbayCount("<html>nope</html>"); err == nil {
		t.Error("expected error when count heading absent")
	}
}

func TestIntentScore(t *testing.T) {
	long := intentScore("printable keto meal prep cookbook", "long-tail")
	short := intentScore("cookbook", "short-tail")
	if long <= short {
		t.Errorf("long-tail+modifiers (%d) should beat bare head term (%d)", long, short)
	}
	if long < 0 || long > 100 || short < 0 || short > 100 {
		t.Errorf("out of range: long=%d short=%d", long, short)
	}
}

func TestSuggestionDemandScore(t *testing.T) {
	if suggestionDemandScore(0) >= suggestionDemandScore(3) {
		t.Error("0 suggestions should score below 3")
	}
	if suggestionDemandScore(3) >= suggestionDemandScore(10) {
		t.Error("demand score should increase with suggestions")
	}
	if v := suggestionDemandScore(100); v != 100 {
		t.Errorf("should clamp to 100, got %d", v)
	}
}

func TestParseSuggestCount(t *testing.T) {
	n, err := parseSuggestCount([]byte(`["keto cookbook",["keto cookbook pdf","keto cookbook printable","keto cookbook free"]]`))
	if err != nil || n != 3 {
		t.Fatalf("parseSuggestCount = %d, %v; want 3", n, err)
	}
	if n, _ := parseSuggestCount([]byte(`["x"]`)); n != 0 {
		t.Errorf("no suggestion array should give 0, got %d", n)
	}
}

func TestCompose(t *testing.T) {
	full := compose(80, 20, 70) // high demand, low comp → strong
	weak := compose(20, 90, 40) // low demand, high comp → poor
	if full <= weak {
		t.Errorf("strong (%d) should beat weak (%d)", full, weak)
	}
	for _, v := range []int{compose(80, 20, 70), compose(80, -1, 70), compose(-1, 20, 70), compose(-1, -1, 70)} {
		if v < 0 || v > 100 {
			t.Errorf("compose out of range: %d", v)
		}
	}
	// With competition unknown, a high-demand phrase still scores well.
	if compose(90, -1, 80) < 60 {
		t.Errorf("demand-only high phrase scored too low: %d", compose(90, -1, 80))
	}
}

func TestEtsyCompetitionStrength(t *testing.T) {
	count := 500
	weakIncumbents := []etsy.Listing{{NumFavorers: 1}, {NumFavorers: 2}, {NumFavorers: 0}}
	strongIncumbents := []etsy.Listing{{NumFavorers: 5000}, {NumFavorers: 8000}, {NumFavorers: 3000}}
	weak := etsyCompetition(count, weakIncumbents)
	strong := etsyCompetition(count, strongIncumbents)
	if strong <= weak {
		t.Errorf("strong incumbents (%d) should raise competition above weak (%d) at equal count", strong, weak)
	}
	// No listings → falls back to count-only score.
	if etsyCompetition(count, nil) != meterFromCount(count) {
		t.Error("empty listings should yield count-only competition")
	}
}
