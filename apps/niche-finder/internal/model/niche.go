// Package model holds the core domain types: a discovered Niche (a named item
// niche) and its list of SEO Keywords, each scored for saturation. Kept separate
// so the store, web, and API layers share one shape.
package model

import (
	"time"

	"github.com/3thanHead/iot_ai/niche-finder/internal/saturation"
)

// Keyword is one SEO search phrase for a niche, with its measured saturation.
type Keyword struct {
	Phrase      string            `json:"phrase"`
	Type        string            `json:"type"` // "long-tail" | "short-tail"
	Saturation  saturation.Result `json:"saturation"`
	Opportunity int               `json:"opportunity"` // 100 - Saturation.Value; higher = better
}

// MeterColor maps a keyword's saturation to a CSS color: green (open) → red.
func (k Keyword) MeterColor() string { return meterColor(k.Saturation.Value) }

// Niche is one discovered item niche: a name + the product framing + a list of
// SEO keywords (long- and short-tail) each scored for how crowded it is.
type Niche struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`         // item niche name, e.g. "Keto meal-prep printable cookbook"
	Theme        string    `json:"theme"`        // seed category / theme it was expanded from
	Audience     string    `json:"audience"`     // who buys
	ProductAngle string    `json:"productAngle"` // the digital/printable product to sell them
	Rationale    string    `json:"rationale"`    // why it's promising (from the LLM)
	Keywords     []Keyword `json:"keywords"`     // associated SEO phrases, scored
	Sources      []string  `json:"sources"`      // lead-source adapters in play
	Opportunity  int       `json:"opportunity"`  // best (highest) keyword opportunity; ranks the niche

	// Management state (set via the dashboard).
	Manual   bool `json:"manual"`   // user-added vs AI-discovered
	Favorite bool `json:"favorite"` // shortlisted
	Archived bool `json:"archived"` // hidden from the default board

	CreatedAt time.Time `json:"createdAt"`
	ScoredAt  time.Time `json:"scoredAt"` // last saturation measurement
}

// BestKeyword returns the lowest-saturation (highest-opportunity) keyword, or a
// zero Keyword when the niche has none.
func (n Niche) BestKeyword() Keyword {
	var best Keyword
	for i, k := range n.Keywords {
		if i == 0 || k.Opportunity > best.Opportunity {
			best = k
		}
	}
	return best
}

func meterColor(v int) string {
	switch {
	case v < 34:
		return "#2e9e5b" // green — low saturation, open lane
	case v < 67:
		return "#d99b1c" // amber — getting crowded
	default:
		return "#cf3b3b" // red — saturated
	}
}
