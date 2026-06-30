package niche

import (
	"context"
	"fmt"
	"strings"

	"github.com/3thanHead/iot_ai/niche-finder/internal/llm"
	"github.com/3thanHead/iot_ai/niche-finder/internal/source"
)

// This file holds the "brains" of the research engine: the LLM phases that turn
// raw leads into investigatable themes, critique the gathered evidence, and
// synthesize grounded niche candidates. The orchestration (fetching, fan-out,
// rounds) lives in finder.go.

// Theme is a line of investigation the LLM proposed from the leads: a label plus
// concrete search terms the engine will dig into. The user never types these —
// the model derives them from what Reddit/Trends surfaced.
type Theme struct {
	Name  string   `json:"name"`
	Terms []string `json:"terms"`
	Note  string   `json:"note"`
}

// KeywordSpec is one SEO phrase the LLM proposed for a niche (before scoring).
type KeywordSpec struct {
	Phrase string `json:"phrase"`
	Type   string `json:"type"` // "long-tail" | "short-tail"
}

// Candidate is one item niche the LLM distilled from a theme's evidence: a name,
// the product framing, and a list of SEO keywords. Saturation is measured later.
type Candidate struct {
	Name         string        `json:"name"`
	Audience     string        `json:"audience"`
	ProductAngle string        `json:"productAngle"`
	Rationale    string        `json:"rationale"`
	Keywords     []KeywordSpec `json:"keywords"`
	Theme        string        `json:"-"` // set by the caller, not the model
}

// Prompt/size budgets.
const (
	sigCharCap     = 220 // truncate each signal line
	maxLeadSignals = 90  // leads fed to theme proposal
	maxThemeSigs   = 45  // evidence fed to per-theme synthesis
	critiqueSample = 6   // evidence lines per theme shown to the critic
	keywordsCap    = 6   // max SEO keywords kept per niche (bounds saturation cost)
)

// productScope frames every prompt: we sell DIGITAL / PRINTABLE products only —
// no physical inventory, no order fulfillment. Keep this consistent everywhere.
const productScope = `We sell DIGITAL and PRINTABLE products only — e.g. printable planners,
cookbooks and recipe cards, wall-art prints, SVG/PNG cut files, journals, templates,
worksheets, digital stickers, e-books, spreadsheets. NO physical products, NO print-on-
demand apparel, NO order fulfillment. Every niche must be deliverable as an instant
digital download.`

// seoRules tells the model the keyword-research conventions to follow.
const seoRules = `Apply SEO / keyword-research rules when writing search terms:
- Prefer LONG-TAIL phrases (3-5 words) with clear BUYER INTENT over broad head terms.
- Use product/format MODIFIERS shoppers type: "printable", "template", "instant download",
  "digital", "bundle", "editable", "svg", "pdf".
- Be specific about audience, occasion, or style (e.g. "minimalist wedding seating chart
  template", not "wedding stuff"). Specific = lower competition + higher conversion.
- Each term should be something a real person would type into Etsy or Google search.`

// proposeThemes turns the inputs into a research agenda. When the user provided
// seed categories, each is expanded into specific sub-niches; otherwise the
// agenda is discovered autonomously from the Reddit/Trends leads. Either way the
// leads are supporting signal, and the model writes the search terms (the user
// never types keywords).
func proposeThemes(ctx context.Context, client *llm.Client, seeds []string, leads []source.Signal, want int) ([]Theme, error) {
	if len(seeds) == 0 && len(leads) == 0 {
		return nil, fmt.Errorf("no seeds or leads to work from")
	}

	var task string
	if len(seeds) > 0 {
		task = fmt.Sprintf(`The user is interested in these high-level categories:
  %s
Expand them into up to %d distinct THEMES (specific sub-niches within those categories).
Use the leads below only as supporting inspiration for what's currently in demand.`,
			strings.Join(seeds, "; "), want)
	} else {
		task = fmt.Sprintf(`Identify up to %d distinct THEMES worth investigating — emerging
interests, hobbies, occasions, or aesthetics hinted at by the leads below.`, want)
	}

	prompt := fmt.Sprintf(`You are a digital-products SEO and market researcher.

%s

%s

%s

For each theme provide:
- name: short label for the sub-niche
- terms: 4-6 long-tail search phrases (per the SEO rules) we can use to dig deeper
- note: one sentence on why it looks promising for digital products

Return JSON: {"themes": [{"name","terms":["..."],"note"}, ...]}

--- LEADS (supporting signal) ---
%s`, productScope, seoRules, task, formatSignals(leads, maxLeadSignals))

	var out struct {
		Themes []Theme `json:"themes"`
	}
	if err := client.CompleteJSON(ctx, prompt, 0.5, &out); err != nil {
		return nil, err
	}
	return cleanThemes(out.Themes), nil
}

// critique reviews the themes and a sample of their gathered evidence, then
// proposes FOLLOW-UP investigations: new angles and new search terms we haven't
// tried. This is the "find the gaps" pass that drives a second deepen round.
func critique(ctx context.Context, client *llm.Client, ev []themeEvidence, usedTerms map[string]bool, want int) ([]Theme, error) {
	var b strings.Builder
	for _, e := range ev {
		fmt.Fprintf(&b, "\n## Theme: %s (%d signals)\n", e.Theme.Name, len(e.Signals))
		b.WriteString(formatSignals(e.Signals, critiqueSample))
	}
	var tried []string
	for t := range usedTerms {
		tried = append(tried, t)
	}

	prompt := fmt.Sprintf(`You are a skeptical research lead reviewing a niche investigation.
Below is what we found per theme. Find the GAPS:
- themes with thin or weak evidence that deserve a different angle,
- promising adjacent niches the evidence hints at but we haven't searched,
- more specific sub-niches worth isolating.

Propose up to %d FOLLOW-UP investigations. For each: name, 3-5 NEW search terms
we have NOT already tried, and a note. Do not repeat any of the already-tried terms.

Already-tried terms: %s

Return JSON: {"themes": [{"name","terms":["..."],"note"}, ...]}

--- EVIDENCE SO FAR ---
%s`, want, strings.Join(tried, ", "), b.String())

	var out struct {
		Themes []Theme `json:"themes"`
	}
	if err := client.CompleteJSON(ctx, prompt, 0.6, &out); err != nil {
		return nil, err
	}
	// Drop any terms the critic reused despite instructions.
	themes := cleanThemes(out.Themes)
	for i := range themes {
		var fresh []string
		for _, t := range themes[i].Terms {
			if !usedTerms[strings.ToLower(strings.TrimSpace(t))] {
				fresh = append(fresh, t)
			}
		}
		themes[i].Terms = fresh
	}
	return cleanThemes(themes), nil
}

// synthesize turns one theme's gathered evidence into concrete niche candidates.
// Evidence may be sparse (especially for seeded themes Reddit doesn't cover) — in
// that case the model falls back to the theme + SEO knowledge with low support.
func synthesize(ctx context.Context, client *llm.Client, e themeEvidence, want int) ([]Candidate, error) {
	evidence := formatSignals(e.Signals, maxThemeSigs)
	if strings.TrimSpace(evidence) == "" {
		evidence = "(no direct search evidence found — propose niches from the theme name and " +
			"SEO knowledge of this category, and set support to 0)"
	}
	prompt := fmt.Sprintf(`You are a digital-products SEO and market researcher.

%s

%s

Theme under investigation: %q.
Below is the evidence we gathered (Reddit posts and searches). Propose up to %d
CONCRETE, SPECIFIC digital-product item niches grounded in the theme and evidence —
not generic guesses. For each niche provide:
- name: the item niche name (short label)
- audience: who buys it
- productAngle: the specific digital/printable product to sell them
- rationale: one sentence citing what supports it
- keywords: 3-6 SEO search phrases for this niche, a MIX of long-tail and short-tail.
  Each is {"phrase","type"} where type is "long-tail" or "short-tail".

Fewer, stronger niches beat padding. Return JSON:
{"niches": [{"name","audience","productAngle","rationale",
  "keywords":[{"phrase","type"}, ...]}, ...]}

--- EVIDENCE ---
%s`, productScope, seoRules, e.Theme.Name, want, evidence)

	var out struct {
		Niches []Candidate `json:"niches"`
	}
	if err := client.CompleteJSON(ctx, prompt, 0.4, &out); err != nil {
		return nil, err
	}
	var clean []Candidate
	for _, c := range out.Niches {
		c.Theme = e.Theme.Name
		if cc, ok := cleanCandidate(c); ok {
			clean = append(clean, cc)
		}
	}
	return clean, nil
}

// cleanCandidate normalises a candidate: requires a name and at least one
// keyword, trims/dedupes phrases, defaults the tail type, and caps the count.
func cleanCandidate(c Candidate) (Candidate, bool) {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return Candidate{}, false
	}
	seen := map[string]bool{}
	var kws []KeywordSpec
	for _, k := range c.Keywords {
		k.Phrase = strings.TrimSpace(k.Phrase)
		if k.Phrase == "" || seen[strings.ToLower(k.Phrase)] {
			continue
		}
		seen[strings.ToLower(k.Phrase)] = true
		k.Type = normalizeTail(k.Type, k.Phrase)
		kws = append(kws, k)
		if len(kws) >= keywordsCap {
			break
		}
	}
	if len(kws) == 0 {
		return Candidate{}, false // a niche with no keywords is useless here
	}
	c.Keywords = kws
	return c, true
}

// normalizeTail maps the model's type to "long-tail"/"short-tail", inferring from
// word count when it's missing or unexpected.
func normalizeTail(t, phrase string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "long-tail", "long", "longtail":
		return "long-tail"
	case "short-tail", "short", "shorttail", "head":
		return "short-tail"
	}
	if len(strings.Fields(phrase)) >= 3 {
		return "long-tail"
	}
	return "short-tail"
}

// enrichKeyword turns a single user-supplied keyword into a full niche profile.
// The model fills in the audience/product/rationale (it does not bypass the AI);
// saturation is measured separately by the caller, like any other niche.
func enrichKeyword(ctx context.Context, client *llm.Client, keyword string) (Candidate, error) {
	prompt := fmt.Sprintf(`You are a digital-products SEO and market researcher.

%s

%s

A user wants to track this niche keyword: %q.
Profile it as a digital-product item niche and expand it into related SEO phrases.
Return JSON:
{"name": "<item niche name>",
 "audience": "<who buys it>",
 "productAngle": "<the specific digital/printable product to sell>",
 "rationale": "<one sentence on the opportunity>",
 "theme": "<the broad category it belongs to>",
 "keywords": [{"phrase","type"}, ...]   // include the user's keyword + 2-5 variations,
                                         // a mix of long-tail and short-tail}`,
		productScope, seoRules, keyword)

	// Theme is json:"-" on Candidate (normally set by the caller), so decode into
	// a local shape that captures the model's theme too.
	var raw struct {
		Candidate
		Theme string `json:"theme"`
	}
	if err := client.CompleteJSON(ctx, prompt, 0.4, &raw); err != nil {
		return Candidate{}, err
	}
	c := raw.Candidate
	c.Theme = raw.Theme
	if strings.TrimSpace(c.Name) == "" {
		c.Name = keyword
	}
	// Guarantee the user's own keyword is present even if the model dropped it.
	c.Keywords = append([]KeywordSpec{{Phrase: keyword}}, c.Keywords...)
	cc, ok := cleanCandidate(c)
	if !ok {
		return Candidate{}, fmt.Errorf("could not profile keyword %q", keyword)
	}
	return cc, nil
}

// cleanThemes drops themes with no name or no terms and trims blanks.
func cleanThemes(in []Theme) []Theme {
	var out []Theme
	for _, t := range in {
		t.Name = strings.TrimSpace(t.Name)
		if t.Name == "" {
			continue
		}
		var terms []string
		for _, term := range t.Terms {
			if term = strings.TrimSpace(term); term != "" {
				terms = append(terms, term)
			}
		}
		if len(terms) == 0 {
			continue
		}
		t.Terms = terms
		out = append(out, t)
	}
	return out
}

// formatSignals renders up to maxN signals as tagged, whitespace-collapsed,
// truncated lines for a prompt.
func formatSignals(signals []source.Signal, maxN int) string {
	var b strings.Builder
	n := 0
	for _, s := range signals {
		if n >= maxN {
			break
		}
		line := strings.TrimSpace(s.Title)
		if s.Text != "" {
			line += " — " + strings.TrimSpace(s.Text)
		}
		line = strings.Join(strings.Fields(line), " ")
		if len(line) > sigCharCap {
			line = line[:sigCharCap]
		}
		if line == "" {
			continue
		}
		fmt.Fprintf(&b, "[%s] %s\n", s.Source, line)
		n++
	}
	return b.String()
}
