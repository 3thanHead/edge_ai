// Package niche turns mined signals into scored niches: it runs the sources,
// asks the LLM to distill candidates, measures each one's saturation, and
// persists the results.
package niche

import (
	"context"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/3thanHead/iot_ai/niche-finder/internal/llm"
	"github.com/3thanHead/iot_ai/niche-finder/internal/model"
	"github.com/3thanHead/iot_ai/niche-finder/internal/saturation"
	"github.com/3thanHead/iot_ai/niche-finder/internal/source"
	"github.com/3thanHead/iot_ai/niche-finder/internal/store"
)

const (
	// scoreConcurrency bounds how many saturation lookups run at once — polite to
	// the marketplaces we query and easy on the LLM fallback.
	scoreConcurrency = 4
	// deepenConcurrency bounds simultaneous Reddit searches; kept low because the
	// public endpoint rate-limits unauthenticated callers.
	deepenConcurrency = 4
	// synthConcurrency bounds simultaneous per-theme synthesis LLM calls.
	synthConcurrency = 3

	// Exhaustive-mode shape (the depth the user chose).
	wantThemes     = 12 // themes proposed from the leads
	followupThemes = 6  // follow-up investigations the critic may add
	searchPerTerm  = 10 // Reddit search results pulled per term
	nichesPerTheme = 2  // niches synthesized per theme
	maxNiches      = 18 // cap saved niches/run (each has several scored keywords)

	// runTimeout caps a single find run so a wedged source/LLM can't run forever.
	// Exhaustive runs do many fetches + LLM calls, so allow generous headroom.
	runTimeout = 12 * time.Minute
)

// Status is a snapshot of the finder for the dashboard. Working counts in-flight
// single-item jobs (manual keyword adds, re-scores) so the UI can show activity.
type Status struct {
	Running    bool
	Stage      string
	StartedAt  time.Time
	FinishedAt time.Time
	Found      int
	Err        string
	Working    int
}

// Finder orchestrates discovery runs and single-item jobs. Seed categories live
// in the store (managed from the dashboard), not on the Finder.
type Finder struct {
	base    context.Context // app root context (cancelled on shutdown)
	sources []source.Source
	llm     *llm.Client
	scorer  *saturation.Scorer
	store   store.Store
	log     *log.Logger

	mu     sync.Mutex
	status Status
}

func NewFinder(base context.Context, sources []source.Source, client *llm.Client, scorer *saturation.Scorer, st store.Store, lg *log.Logger) *Finder {
	return &Finder{base: base, sources: sources, llm: client, scorer: scorer, store: st, log: lg}
}

// Status returns a copy of the current run status (safe for the web layer).
func (f *Finder) Status() Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status
}

// Start launches a discovery run in the background if one isn't already going.
// category limits the run to one seed category; empty runs across all seeds.
// Returns false if a run is already in progress.
func (f *Finder) Start(category string) bool {
	f.mu.Lock()
	if f.status.Running {
		f.mu.Unlock()
		return false
	}
	f.status = Status{Running: true, Stage: "mining leads", StartedAt: time.Now(), Working: f.status.Working}
	f.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(f.base, runTimeout)
		defer cancel()
		f.run(ctx, category)
	}()
	return true
}

// bump adjusts the in-flight single-item job counter.
func (f *Finder) bump(delta int) {
	f.mu.Lock()
	f.status.Working += delta
	f.mu.Unlock()
}

// resolveSeeds returns the seeds for a run: just `category` if given, else all
// managed seeds from the store.
func (f *Finder) resolveSeeds(category string) []string {
	if strings.TrimSpace(category) != "" {
		return []string{category}
	}
	seeds, err := f.store.Seeds()
	if err != nil {
		f.log.Printf("read seeds: %v", err)
	}
	return seeds
}

func (f *Finder) setStage(stage string) {
	f.mu.Lock()
	f.status.Stage = stage
	f.mu.Unlock()
}

func (f *Finder) finish(found int, err error) {
	f.mu.Lock()
	f.status.Running = false
	f.status.Stage = "idle"
	f.status.FinishedAt = time.Now()
	f.status.Found = found
	if err != nil {
		f.status.Err = err.Error()
	} else {
		f.status.Err = ""
	}
	f.mu.Unlock()
}

// themeEvidence is a theme plus everything the deepen phase gathered for it.
type themeEvidence struct {
	Theme   Theme
	Signals []source.Signal
}

// run executes the guided research pipeline:
//
//	mine leads → propose themes → deepen (search) → critique → deepen again
//	→ synthesize per theme → score saturation → save
//
// The LLM proposes the search terms from the leads, so no keywords are required
// from the user. It records the outcome (and live stage) in status.
func (f *Finder) run(ctx context.Context, category string) {
	seedList := f.resolveSeeds(category)

	// Phase 1 — leads (Reddit hot + Google Trends).
	leads := f.mineAll(ctx)
	f.log.Printf("mined %d lead signals from %d sources (seeds=%d)", len(leads), len(f.sources), len(seedList))

	// Phase 2 — turn the seeds + leads into a research agenda.
	f.setStage("proposing themes")
	themes, err := proposeThemes(ctx, f.llm, seedList, leads, wantThemes)
	if err != nil {
		f.log.Printf("propose themes: %v", err)
		f.finish(0, err)
		return
	}
	f.log.Printf("proposed %d themes", len(themes))

	// Phase 3 — deepen each theme by searching its terms (round 1).
	seen := map[string]bool{} // every search term tried, to avoid repeats
	f.setStage("deepening (round 1)")
	ev := f.deepenThemes(ctx, themes, seen)

	// Phase 4 — critic finds gaps and proposes fresh follow-up investigations.
	f.setStage("critiquing")
	follow, cerr := critique(ctx, f.llm, ev, seen, followupThemes)
	if cerr != nil {
		f.log.Printf("critique: %v", cerr) // non-fatal; we still have round 1
	}
	if len(follow) > 0 {
		f.setStage("deepening (round 2)")
		ev = mergeEvidence(ev, f.deepenThemes(ctx, follow, seen))
		f.log.Printf("critic added %d follow-up themes", len(follow))
	}

	// Phase 5 — synthesize concrete niches from each theme's evidence.
	f.setStage("synthesizing niches")
	candidates := f.synthesizeAll(ctx, ev)
	f.log.Printf("synthesized %d niche candidates across %d themes", len(candidates), len(ev))
	if len(candidates) > maxNiches {
		candidates = candidates[:maxNiches]
	}

	// Phase 6 — measure saturation and persist.
	f.setStage("scoring saturation")
	niches := f.scoreAll(ctx, candidates)
	for _, n := range niches {
		if err := f.store.Save(n); err != nil {
			f.log.Printf("save %s: %v", n.Name, err)
		}
	}
	f.log.Printf("saved %d niches", len(niches))
	f.finish(len(niches), nil)
}

// deepenThemes searches each theme's terms on Reddit (bounded concurrency) and
// returns the gathered evidence per theme. `seen` is updated with every term
// tried so later rounds don't repeat searches.
func (f *Finder) deepenThemes(ctx context.Context, themes []Theme, seen map[string]bool) []themeEvidence {
	ev := make([]themeEvidence, len(themes))
	sigs := make([][]source.Signal, len(themes))

	type task struct {
		ti   int
		term string
	}
	var tasks []task
	for ti, th := range themes {
		ev[ti] = themeEvidence{Theme: th}
		for _, term := range th.Terms {
			key := strings.ToLower(strings.TrimSpace(term))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true // mutated sequentially here, before any goroutine
			tasks = append(tasks, task{ti, term})
		}
	}

	sem := make(chan struct{}, deepenConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, t := range tasks {
		wg.Add(1)
		go func(t task) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			got, err := source.SearchReddit(ctx, t.term, searchPerTerm)
			if err != nil {
				f.log.Printf("search %q: %v", t.term, err)
				return
			}
			mu.Lock()
			sigs[t.ti] = append(sigs[t.ti], got...)
			mu.Unlock()
		}(t)
	}
	wg.Wait()

	for ti := range ev {
		ev[ti].Signals = sigs[ti]
	}
	return ev
}

// mergeEvidence folds follow-up evidence into the base, consolidating by theme
// name so a critic-revisited theme accumulates signals rather than duplicating.
func mergeEvidence(base, extra []themeEvidence) []themeEvidence {
	idx := make(map[string]int, len(base))
	for i, e := range base {
		idx[strings.ToLower(e.Theme.Name)] = i
	}
	for _, e := range extra {
		k := strings.ToLower(e.Theme.Name)
		if i, ok := idx[k]; ok {
			base[i].Signals = append(base[i].Signals, e.Signals...)
			base[i].Theme.Terms = append(base[i].Theme.Terms, e.Theme.Terms...)
		} else {
			idx[k] = len(base)
			base = append(base, e)
		}
	}
	return base
}

// synthesizeAll turns each theme's evidence into niche candidates concurrently.
func (f *Finder) synthesizeAll(ctx context.Context, ev []themeEvidence) []Candidate {
	out := make([][]Candidate, len(ev))
	sem := make(chan struct{}, synthConcurrency)
	var wg sync.WaitGroup
	for i, e := range ev {
		wg.Add(1)
		go func(i int, e themeEvidence) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			c, err := synthesize(ctx, f.llm, e, nichesPerTheme)
			if err != nil {
				f.log.Printf("synthesize %q: %v", e.Theme.Name, err)
				return
			}
			out[i] = c
		}(i, e)
	}
	wg.Wait()
	var all []Candidate
	for _, c := range out {
		all = append(all, c...)
	}
	return all
}

// mineAll runs every source concurrently and pools their signals; a failing
// source is logged and skipped.
func (f *Finder) mineAll(ctx context.Context) []source.Signal {
	var (
		mu      sync.Mutex
		signals []source.Signal
		wg      sync.WaitGroup
	)
	for _, s := range f.sources {
		wg.Add(1)
		go func(s source.Source) {
			defer wg.Done()
			got, err := s.Mine(ctx)
			if err != nil {
				f.log.Printf("source %s: %v", s.Name(), err)
			}
			mu.Lock()
			signals = append(signals, got...)
			mu.Unlock()
		}(s)
	}
	wg.Wait()
	return signals
}

// scoreAll measures saturation for each candidate (bounded concurrency) and
// builds the persisted niche records.
func (f *Finder) scoreAll(ctx context.Context, candidates []Candidate) []*model.Niche {
	out := make([]*model.Niche, len(candidates))
	sem := make(chan struct{}, scoreConcurrency)
	var wg sync.WaitGroup
	for i, c := range candidates {
		wg.Add(1)
		go func(i int, c Candidate) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out[i] = f.buildNiche(ctx, c)
		}(i, c)
	}
	wg.Wait()
	// Drop any nil holes (shouldn't happen, but be safe).
	var niches []*model.Niche
	for _, n := range out {
		if n != nil {
			niches = append(niches, n)
		}
	}
	return niches
}

// buildNiche scores every keyword on a candidate and assembles the record. The
// niche's opportunity is its best (least-saturated) keyword.
func (f *Finder) buildNiche(ctx context.Context, c Candidate) *model.Niche {
	now := time.Now()
	n := &model.Niche{
		ID:           uuid.NewString(),
		Name:         c.Name,
		Theme:        c.Theme,
		Audience:     c.Audience,
		ProductAngle: c.ProductAngle,
		Rationale:    c.Rationale,
		Sources:      sourceNames(f.sources),
		CreatedAt:    now,
		ScoredAt:     now,
	}
	for _, k := range c.Keywords {
		n.Keywords = append(n.Keywords, f.scoreKeyword(ctx, k.Phrase, k.Type))
	}
	rankKeywords(n)
	return n
}

// scoreKeyword measures one phrase's saturation.
func (f *Finder) scoreKeyword(ctx context.Context, phrase, tail string) model.Keyword {
	sat := f.scorer.Score(ctx, phrase)
	return model.Keyword{
		Phrase:      phrase,
		Type:        tail,
		Saturation:  sat,
		Opportunity: 100 - sat.Value,
	}
}

// rankKeywords sorts a niche's keywords best-opportunity-first and sets the
// niche-level opportunity to the best of them.
func rankKeywords(n *model.Niche) {
	sort.SliceStable(n.Keywords, func(i, j int) bool {
		return n.Keywords[i].Opportunity > n.Keywords[j].Opportunity
	})
	n.Opportunity = n.BestKeyword().Opportunity
}

// AddKeyword enriches a user-supplied keyword into a full niche (the LLM fills in
// audience/product/rationale — it does NOT bypass the AI) and scores its
// saturation the same way discovered niches are scored. Runs in the background.
func (f *Finder) AddKeyword(keyword string) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return
	}
	f.bump(1)
	go func() {
		defer f.bump(-1)
		ctx, cancel := context.WithTimeout(f.base, 2*time.Minute)
		defer cancel()

		c, err := enrichKeyword(ctx, f.llm, keyword)
		if err != nil {
			f.log.Printf("enrich %q: %v", keyword, err) // fall back to the bare keyword
			c = Candidate{Name: keyword, Keywords: []KeywordSpec{{Phrase: keyword, Type: "long-tail"}}}
		}
		n := f.buildNiche(ctx, c)
		n.Manual = true
		if err := f.store.Save(n); err != nil {
			f.log.Printf("save manual %q: %v", keyword, err)
		}
	}()
}

// Rescore re-measures saturation for an existing niche (competition shifts over
// time). Runs in the background.
func (f *Finder) Rescore(id string) {
	f.bump(1)
	go func() {
		defer f.bump(-1)
		ctx, cancel := context.WithTimeout(f.base, 2*time.Minute)
		defer cancel()

		n, err := f.store.Get(id)
		if err != nil {
			f.log.Printf("rescore: get %s: %v", id, err)
			return
		}
		for i, k := range n.Keywords {
			n.Keywords[i] = f.scoreKeyword(ctx, k.Phrase, k.Type)
		}
		rankKeywords(n)
		n.ScoredAt = time.Now()
		if err := f.store.Save(n); err != nil {
			f.log.Printf("rescore: save %s: %v", id, err)
		}
	}()
}

func sourceNames(sources []source.Source) []string {
	names := make([]string, 0, len(sources))
	for _, s := range sources {
		names = append(names, s.Name())
	}
	return names
}
