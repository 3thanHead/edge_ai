// Package saturation scores a keyword's *opportunity* (0-100, higher = better) as
// a composite of three free signals: search DEMAND (Google autocomplete depth),
// COMPETITION (Etsy listing count + incumbent strength, with eBay/LLM fallback),
// and buyer INTENT (phrase heuristics). Opportunity rewards high demand, low
// competition, and strong intent — a far better gauge than raw listing count.
package saturation

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/3thanHead/iot_ai/niche-finder/internal/etsy"
)

// Score is a keyword's opportunity plus the components that produced it. A
// component of -1 means "couldn't measure" (e.g. competition with no Etsy key).
type Score struct {
	Opportunity int    `json:"opportunity"` // composite 0-100, higher = better
	Demand      int    `json:"demand"`      // 0-100 search demand (-1 unknown)
	Competition int    `json:"competition"` // 0-100 saturation, higher = crowded (-1 unknown)
	Intent      int    `json:"intent"`      // 0-100 buyer intent
	Competitors int    `json:"competitors,omitempty"`
	DemandSrc   string `json:"demandSource,omitempty"`
	CompSrc     string `json:"compSource,omitempty"`
	Confidence  string `json:"confidence"` // "high" (real demand+competition) | "low"
	Detail      string `json:"detail"`
}

// JSONEstimator is the slice of the LLM client used for the competition fallback.
type JSONEstimator interface {
	CompleteJSON(ctx context.Context, prompt string, temperature float64, out any) error
}

// Scorer computes opportunity. markets is the competition try-order (e.g.
// ["etsy"]); etsy is nil without an API key; llm is the last-resort estimator.
type Scorer struct {
	markets []string
	etsy    *etsy.Client
	llm     JSONEstimator
}

func NewScorer(markets []string, etsyClient *etsy.Client, llm JSONEstimator) *Scorer {
	return &Scorer{markets: markets, etsy: etsyClient, llm: llm}
}

// composite weights (must each sum to 1 within a branch).
const (
	wDemand = 0.40
	wComp   = 0.40
	wIntent = 0.20
)

// Score computes a keyword's opportunity. It never errors: unmeasurable signals
// drop out and the remaining ones are reweighted, with Confidence marked "low".
func (s *Scorer) Score(ctx context.Context, phrase, tail string) Score {
	intent := intentScore(phrase, tail)
	demand, suggestions, derr := autocompleteDemand(ctx, phrase)
	if derr != nil {
		demand = -1
	}
	comp, competitors, compSrc := s.competition(ctx, phrase)

	sc := Score{
		Demand:      demand,
		Competition: comp,
		Intent:      intent,
		Competitors: competitors,
		CompSrc:     compSrc,
	}
	if demand >= 0 {
		sc.DemandSrc = "google-autocomplete"
	}
	sc.Opportunity = compose(demand, comp, intent)
	// High confidence only when both demand and a *measured* competition exist
	// (an LLM-estimated competition is still a guess).
	if demand >= 0 && compSrc == "etsy" {
		sc.Confidence = "high"
	} else {
		sc.Confidence = "low"
	}
	sc.Detail = detail(demand, suggestions, comp, competitors, compSrc, intent)
	return sc
}

// compose blends the available signals into a 0-100 opportunity, reweighting when
// demand or competition is missing.
func compose(demand, comp, intent int) int {
	switch {
	case demand >= 0 && comp >= 0:
		v := wDemand*f(demand) + wComp*f(100-comp) + wIntent*f(intent)
		return clamp(round(v), 0, 100)
	case demand >= 0: // no competition signal
		v := 0.60*f(demand) + 0.40*f(intent)
		return clamp(round(v), 0, 100)
	case comp >= 0: // no demand signal
		v := 0.55*f(100-comp) + 0.45*f(intent)
		return clamp(round(v), 0, 100)
	default: // only intent
		return clamp(intent, 0, 100)
	}
}

func detail(demand, suggestions, comp, competitors int, compSrc string, intent int) string {
	d := "demand n/a"
	if demand >= 0 {
		d = fmt.Sprintf("demand %d (%d autocompletes)", demand, suggestions)
	}
	c := "competition n/a"
	if comp >= 0 {
		switch {
		case competitors > 0:
			c = fmt.Sprintf("competition %d (%s, %d listings)", comp, compSrc, competitors)
		default:
			c = fmt.Sprintf("competition %d (%s)", comp, compSrc)
		}
	}
	return strings.Join([]string{d, c, fmt.Sprintf("intent %d", intent)}, " · ")
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

// logRatio returns log10(n) / log10(ref) — a 0..1 (and beyond) position of n on a
// log scale anchored at ref.
func logRatio(n, ref int) float64 {
	return math.Log10(float64(n)) / math.Log10(float64(ref))
}

func f(v int) float64     { return float64(v) }
func round(v float64) int { return int(math.Round(v)) }

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
