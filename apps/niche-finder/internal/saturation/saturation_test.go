package saturation

import (
	"context"
	"encoding/json"
	"testing"
)

func TestMeterFromCount(t *testing.T) {
	cases := []struct {
		count   int
		wantMin int
		wantMax int
	}{
		{0, 0, 0},           // nothing competing → wide open
		{10, 15, 30},        // ~21
		{1000, 55, 65},      // ~60
		{100_000, 100, 100}, // reference → fully saturated
		{500_000, 100, 100}, // clamped
	}
	for _, c := range cases {
		got := meterFromCount(c.count)
		if got < c.wantMin || got > c.wantMax {
			t.Errorf("meterFromCount(%d) = %d, want within [%d,%d]", c.count, got, c.wantMin, c.wantMax)
		}
	}
}

func TestMeterMonotonic(t *testing.T) {
	prev := -1
	for _, n := range []int{0, 5, 50, 500, 5000, 50000, 100000} {
		v := meterFromCount(n)
		if v < prev {
			t.Errorf("meter not monotonic: count=%d gave %d after %d", n, v, prev)
		}
		prev = v
	}
}

func TestParseEbayCount(t *testing.T) {
	html := `<div><h1 class="srp-controls__count-heading"><span class="BOLD">12,345</span> results for vintage map</h1></div>`
	got, err := parseEbayCount(html)
	if err != nil {
		t.Fatalf("parseEbayCount: %v", err)
	}
	if got != 12345 {
		t.Errorf("parseEbayCount = %d, want 12345", got)
	}
}

func TestParseEbayCountMissing(t *testing.T) {
	if _, err := parseEbayCount("<html>no results block here</html>"); err == nil {
		t.Error("expected error when count heading absent")
	}
}

// stubLLM lets us exercise the estimate fallback without a network/model: it
// unmarshals a fixed JSON payload into the caller's out value.
type stubLLM struct{ payload string }

func (s stubLLM) CompleteJSON(_ context.Context, _ string, _ float64, out any) error {
	return json.Unmarshal([]byte(s.payload), out)
}

// With no measurable markets, Score must fall through to the LLM estimate.
func TestScoreFallsBackToEstimate(t *testing.T) {
	s := NewScorer([]string{"etsy"}, nil, stubLLM{`{"value":77,"rationale":"crowded"}`})
	got := s.Score(context.Background(), "vintage map")
	if got.Method != "estimated" || got.Source != "llm" {
		t.Errorf("method/source = %q/%q, want estimated/llm", got.Method, got.Source)
	}
	if got.Value != 77 {
		t.Errorf("value = %d, want 77", got.Value)
	}
}

// A nil LLM with nothing measurable yields a neutral, labelled default.
func TestScoreNeutralWhenNoLLM(t *testing.T) {
	s := NewScorer([]string{"etsy"}, nil, nil)
	got := s.Score(context.Background(), "x")
	if got.Method != "estimated" || got.Value != 50 {
		t.Errorf("got %+v, want neutral estimated 50", got)
	}
}
