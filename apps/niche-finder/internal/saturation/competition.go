package saturation

import (
	"context"
	"fmt"
	"sort"

	"github.com/3thanHead/iot_ai/niche-finder/internal/etsy"
)

// competition returns a 0-100 saturation score (higher = more crowded), the
// competitor count, and the source. It tries the configured markets in order;
// Etsy (when keyed) blends listing *count* with incumbent *strength* (how many
// favorites the top listings have). Returns (-1, 0, "none") when unmeasurable —
// the composite then leans on demand + intent and marks confidence "low".
func (s *Scorer) competition(ctx context.Context, phrase string) (int, int, string) {
	for _, m := range s.markets {
		switch m {
		case "etsy":
			if s.etsy == nil {
				continue
			}
			count, listings, err := s.etsy.ActiveListings(ctx, phrase, 20)
			if err != nil {
				continue
			}
			return etsyCompetition(count, listings), count, "etsy"
		case "ebay":
			if count, err := ebayCount(ctx, phrase); err == nil {
				return meterFromCount(count), count, "ebay"
			}
		}
	}
	if v, ok := s.llmCompetition(ctx, phrase); ok {
		return v, 0, "llm"
	}
	return -1, 0, "none"
}

// etsyCompetition blends listing count (supply) with incumbent strength (the
// median favorites of the top listings). A market with many listings but weak
// incumbents is less saturated than the count alone suggests, and vice-versa.
func etsyCompetition(count int, top []etsy.Listing) int {
	countScore := meterFromCount(count)
	strength := strengthScore(top)
	if strength < 0 {
		return countScore
	}
	return clamp(round(0.6*f(countScore)+0.4*f(strength)), 0, 100)
}

// strengthScore maps the median num_favorers of the top listings to 0-100 on a
// log scale. Returns -1 when there's nothing to measure.
func strengthScore(top []etsy.Listing) int {
	if len(top) == 0 {
		return -1
	}
	favs := make([]int, 0, len(top))
	for _, l := range top {
		favs = append(favs, l.NumFavorers)
	}
	sort.Ints(favs)
	median := favs[len(favs)/2]
	return favStrength(median)
}

// favStrengthRef is the median-favorites count treated as "fully entrenched".
const favStrengthRef = 2_000

func favStrength(median int) int {
	if median <= 0 {
		return 0
	}
	v := 100 * logRatio(median+1, favStrengthRef)
	return clamp(round(v), 0, 100)
}

// llmCompetition asks the LLM to estimate saturation (0-100) as a last resort.
func (s *Scorer) llmCompetition(ctx context.Context, phrase string) (int, bool) {
	if s.llm == nil {
		return 0, false
	}
	var out struct {
		Value int `json:"value"`
	}
	prompt := fmt.Sprintf(`Estimate how saturated the market is for the digital-product search term %q.
Return JSON: {"value": <0-100 integer, 0 = wide open, 100 = totally saturated>}.`, phrase)
	if err := s.llm.CompleteJSON(ctx, prompt, 0.2, &out); err != nil {
		return 0, false
	}
	return clamp(out.Value, 0, 100), true
}
