// Package saturation scores how crowded a niche is on a 0-100 meter (0 = wide
// open, 100 = saturated). It prefers a real competition count from a marketplace
// (Etsy's official API, else an eBay search-result count) and falls back to an
// LLM estimate when no count can be measured.
package saturation

import (
	"context"
	"fmt"
	"math"

	"github.com/3thanHead/iot_ai/niche-finder/internal/etsy"
)

// Result is a single niche's saturation score plus how it was derived.
type Result struct {
	Value       int    `json:"value"`       // 0-100 saturation meter
	Method      string `json:"method"`      // "measured" | "estimated"
	Competitors int    `json:"competitors"` // listing count, when measured
	Source      string `json:"source"`      // "etsy" | "ebay" | "llm"
	Detail      string `json:"detail"`      // human note for the UI
}

// JSONEstimator is the slice of the LLM client used for the fallback estimate.
type JSONEstimator interface {
	CompleteJSON(ctx context.Context, prompt string, temperature float64, out any) error
}

// Scorer measures saturation across configured markets, in order.
type Scorer struct {
	markets []string      // try order, e.g. ["etsy","ebay"]
	etsy    *etsy.Client  // nil when no API key
	llm     JSONEstimator // fallback
}

func NewScorer(markets []string, etsyClient *etsy.Client, llm JSONEstimator) *Scorer {
	return &Scorer{markets: markets, etsy: etsyClient, llm: llm}
}

// Score returns the saturation of `keyword`. It never returns an error: if every
// measured source fails it returns an LLM estimate, and if that fails too it
// returns a neutral, clearly-labelled unknown.
func (s *Scorer) Score(ctx context.Context, keyword string) Result {
	for _, m := range s.markets {
		switch m {
		case "etsy":
			if s.etsy == nil {
				continue
			}
			count, _, err := s.etsy.ActiveListings(ctx, keyword, 1)
			if err == nil {
				return measured("etsy", count)
			}
		case "ebay":
			if count, err := ebayCount(ctx, keyword); err == nil {
				return measured("ebay", count)
			}
		}
	}
	return s.estimate(ctx, keyword)
}

// measured builds a Result from a real competitor count.
func measured(source string, count int) Result {
	return Result{
		Value:       meterFromCount(count),
		Method:      "measured",
		Competitors: count,
		Source:      source,
		Detail:      fmt.Sprintf("%d competing listings on %s", count, source),
	}
}

// saturationRef is the listing count treated as "fully saturated" (meter 100).
const saturationRef = 100_000

// meterFromCount maps a competitor count to 0-100 on a log scale, so the meter
// stays meaningful across the huge range of niche sizes (10 vs 10,000 listings).
func meterFromCount(count int) int {
	if count <= 0 {
		return 0
	}
	v := 100 * math.Log10(float64(count)+1) / math.Log10(saturationRef)
	return clamp(int(math.Round(v)), 0, 100)
}

// estimate asks the LLM to guess saturation when nothing could be measured.
func (s *Scorer) estimate(ctx context.Context, keyword string) Result {
	if s.llm == nil {
		return Result{Value: 50, Method: "estimated", Source: "none", Detail: "no data; neutral default"}
	}
	var out struct {
		Value     int    `json:"value"`
		Rationale string `json:"rationale"`
	}
	prompt := fmt.Sprintf(`Estimate how saturated the print-on-demand / e-commerce market is for the niche %q.
Return a JSON object: {"value": <0-100 integer, 0 = wide open, 100 = totally saturated>, "rationale": "<one short sentence>"}.`, keyword)
	if err := s.llm.CompleteJSON(ctx, prompt, 0.2, &out); err != nil {
		return Result{Value: 50, Method: "estimated", Source: "llm", Detail: "LLM estimate failed; neutral default"}
	}
	detail := out.Rationale
	if detail == "" {
		detail = "LLM estimate (no live competition data)"
	}
	return Result{Value: clamp(out.Value, 0, 100), Method: "estimated", Source: "llm", Detail: detail}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
