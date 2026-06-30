package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/3thanHead/iot_ai/niche-finder/internal/model"
)

// The niche-finder exposes its results for other tools to consume:
//   GET /api/niches            JSON (default)
//   GET /api/niches?format=text   plain text
//   GET /api/niches.txt        plain text
// Filters: ?category=<seed> (match theme), ?show=all (include archived).

// apiKeyword is the stable wire shape of one scored keyword.
type apiKeyword struct {
	Phrase      string `json:"phrase"`
	Type        string `json:"type"`
	Saturation  int    `json:"saturation"`
	Opportunity int    `json:"opportunity"`
	Method      string `json:"method"`
	Competitors int    `json:"competitors,omitempty"`
	Source      string `json:"source,omitempty"`
}

// apiNiche is the stable wire shape of one niche.
type apiNiche struct {
	Name        string       `json:"name"`
	Category    string       `json:"category"`
	Audience    string       `json:"audience,omitempty"`
	Product     string       `json:"product,omitempty"`
	Rationale   string       `json:"rationale,omitempty"`
	Opportunity int          `json:"opportunity"`
	Favorite    bool         `json:"favorite,omitempty"`
	Manual      bool         `json:"manual,omitempty"`
	Keywords    []apiKeyword `json:"keywords"`
}

func (s *Server) apiNiches(w http.ResponseWriter, r *http.Request) {
	niches, err := s.collect(r.URL.Query().Get("show") == "all", r.URL.Query().Get("category"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	text := strings.HasSuffix(r.URL.Path, ".txt") || r.URL.Query().Get("format") == "text"
	if text {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(renderText(niches)))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(niches); err != nil {
		s.log.Printf("api encode: %v", err)
	}
}

// collect loads niches, applies the archived/category filters, and maps them to
// the API shape (already ranked best-opportunity-first by the store).
func (s *Server) collect(showAll bool, category string) ([]apiNiche, error) {
	all, err := s.st.List()
	if err != nil {
		return nil, err
	}
	cat := strings.ToLower(strings.TrimSpace(category))
	out := make([]apiNiche, 0, len(all))
	for _, n := range all {
		if n.Archived && !showAll {
			continue
		}
		if cat != "" && strings.ToLower(n.Theme) != cat {
			continue
		}
		out = append(out, toAPI(n))
	}
	return out, nil
}

func toAPI(n *model.Niche) apiNiche {
	a := apiNiche{
		Name:        n.Name,
		Category:    n.Theme,
		Audience:    n.Audience,
		Product:     n.ProductAngle,
		Rationale:   n.Rationale,
		Opportunity: n.Opportunity,
		Favorite:    n.Favorite,
		Manual:      n.Manual,
	}
	for _, k := range n.Keywords {
		a.Keywords = append(a.Keywords, apiKeyword{
			Phrase:      k.Phrase,
			Type:        k.Type,
			Saturation:  k.Saturation.Value,
			Opportunity: k.Opportunity,
			Method:      k.Saturation.Method,
			Competitors: k.Saturation.Competitors,
			Source:      k.Saturation.Source,
		})
	}
	return a
}

// renderText produces a readable plain-text report of the niches.
func renderText(niches []apiNiche) string {
	var b strings.Builder
	fmt.Fprintf(&b, "niche-finder — %d niches\n", len(niches))
	for _, n := range niches {
		cat := n.Category
		if cat == "" {
			cat = "—"
		}
		fmt.Fprintf(&b, "\n[%s] %s  (best opp %d)\n", cat, n.Name, n.Opportunity)
		if n.Audience != "" {
			fmt.Fprintf(&b, "  audience: %s\n", n.Audience)
		}
		if n.Product != "" {
			fmt.Fprintf(&b, "  product:  %s\n", n.Product)
		}
		b.WriteString("  keywords:\n")
		for _, k := range n.Keywords {
			extra := k.Method
			if k.Competitors > 0 {
				extra = fmt.Sprintf("%s, %d on %s", k.Method, k.Competitors, k.Source)
			}
			fmt.Fprintf(&b, "    [%-10s] %-45s sat %3d  opp %3d  (%s)\n",
				k.Type, k.Phrase, k.Saturation, k.Opportunity, extra)
		}
	}
	return b.String()
}
