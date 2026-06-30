package source

import (
	"context"
	"strings"

	"github.com/3thanHead/iot_ai/niche-finder/internal/etsy"
)

// Etsy mines active-listing titles/tags from the official Etsy Open API v3. It
// has no "trending" feed, so it probes a few broad seed queries and lets the LLM
// spot patterns in the popular-product language that comes back. (The same API's
// `count` field powers the saturation meter — see the saturation package.)
type Etsy struct {
	client *etsy.Client
	seeds  []string
	limit  int
}

// etsySeeds are broad, high-traffic queries whose top listings reveal the
// vocabulary of what currently sells on Etsy.
var etsySeeds = []string{"personalized gift", "custom", "trending", "bestseller", "handmade"}

func NewEtsy(apiKey string, limit int) *Etsy {
	return &Etsy{client: etsy.New(apiKey), seeds: etsySeeds, limit: limit}
}

func (e *Etsy) Name() string { return "etsy" }

func (e *Etsy) Mine(ctx context.Context) ([]Signal, error) {
	per := e.limit / max(len(e.seeds), 1)
	if per < 1 {
		per = 1
	}
	var signals []Signal
	var firstErr error
	for _, seed := range e.seeds {
		_, listings, err := e.client.ActiveListings(ctx, seed, per)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, l := range listings {
			if l.Title == "" {
				continue
			}
			signals = append(signals, Signal{
				Source: "etsy:" + seed,
				Title:  l.Title,
				Text:   strings.Join(l.Tags, ", "),
				URL:    l.URL,
			})
		}
	}
	if len(signals) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return signals, nil
}
