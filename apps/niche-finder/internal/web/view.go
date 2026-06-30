package web

import "github.com/3thanHead/iot_ai/niche-finder/internal/model"

// keywordView is a template-friendly projection of one scored keyword.
type keywordView struct {
	Phrase      string
	Type        string // "long-tail" | "short-tail"
	Opportunity int
	MeterColor  string
	Demand      int
	Competition int
	Intent      int
	Confidence  string // "high" | "low"
	Detail      string
}

// nicheView is a template-friendly projection of a niche and its keyword list.
type nicheView struct {
	ID           string
	Name         string
	Theme        string
	Audience     string
	ProductAngle string
	Rationale    string
	Opportunity  int
	Keywords     []keywordView
	Manual       bool
	Favorite     bool
	Archived     bool
}

func toView(n *model.Niche) nicheView {
	v := nicheView{
		ID:           n.ID,
		Name:         n.Name,
		Theme:        n.Theme,
		Audience:     n.Audience,
		ProductAngle: n.ProductAngle,
		Rationale:    n.Rationale,
		Opportunity:  n.Opportunity,
		Manual:       n.Manual,
		Favorite:     n.Favorite,
		Archived:     n.Archived,
	}
	for _, k := range n.Keywords {
		v.Keywords = append(v.Keywords, keywordView{
			Phrase:      k.Phrase,
			Type:        k.Type,
			Opportunity: k.Opportunity,
			MeterColor:  k.MeterColor(),
			Demand:      k.Score.Demand,
			Competition: k.Score.Competition,
			Intent:      k.Score.Intent,
			Confidence:  k.Score.Confidence,
			Detail:      k.Score.Detail,
		})
	}
	return v
}
