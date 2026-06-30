package saturation

import "strings"

// Intent rates how strongly a phrase signals a ready-to-buy digital-product
// shopper — free, no network. Long-tail, specific, modifier-rich phrases convert
// better and usually face weaker competition than broad head terms.

// buyerModifiers are format/intent words digital-product shoppers actually type.
var buyerModifiers = []string{
	"printable", "template", "instant download", "digital download", "digital",
	"editable", "svg", "png", "pdf", "bundle", "cricut", "planner", "worksheet",
	"kit", "pack", "set", "personalized", "custom", "wall art", "poster",
}

// intentScore returns a 0-100 buyer-intent score from phrase specificity (word
// count), commercial modifiers, and the long/short-tail type.
func intentScore(phrase, tail string) int {
	p := strings.ToLower(phrase)
	score := 35

	switch words := len(strings.Fields(p)); {
	case words >= 5:
		score += 30
	case words >= 3:
		score += 20
	case words >= 2:
		score += 8
	}

	hits := 0
	for _, m := range buyerModifiers {
		if strings.Contains(p, m) {
			hits++
		}
	}
	score += hits * 8

	if strings.EqualFold(strings.TrimSpace(tail), "long-tail") {
		score += 8
	}
	return clamp(score, 0, 100)
}
