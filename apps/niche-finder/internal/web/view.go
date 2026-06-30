package web

import "github.com/3thanHead/iot_ai/niche-finder/internal/model"

// keywordView is a template-friendly projection of one scored keyword.
type keywordView struct {
	Phrase      string
	Type        string // "long-tail" | "short-tail"
	Saturation  int
	Opportunity int
	MeterColor  string
	Method      string // "measured" | "estimated"
	Competitors int
	SatSource   string
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
			Saturation:  k.Saturation.Value,
			Opportunity: k.Opportunity,
			MeterColor:  k.MeterColor(),
			Method:      k.Saturation.Method,
			Competitors: k.Saturation.Competitors,
			SatSource:   k.Saturation.Source,
			Detail:      k.Saturation.Detail,
		})
	}
	return v
}
