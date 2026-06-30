// Package web serves the niche dashboard: managed seed categories, a board of
// discovered/added niche keywords each with a saturation meter, and the controls
// to run discovery, add keywords, and curate the board.
package web

import (
	"embed"
	"html/template"
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/3thanHead/iot_ai/niche-finder/internal/model"
	"github.com/3thanHead/iot_ai/niche-finder/internal/niche"
	"github.com/3thanHead/iot_ai/niche-finder/internal/store"
)

//go:embed templates/*.html
var tmplFS embed.FS

// Runner is the slice of the finder the dashboard drives.
type Runner interface {
	Start(category string) bool // discovery run; "" = all seeds
	Status() niche.Status
	AddKeyword(keyword string) // enrich + score a user keyword
	Rescore(id string)         // re-measure one niche's saturation
}

// Server wires HTTP handlers to the store (reads + curation) and finder (jobs).
type Server struct {
	model  string // LLM tag, for display
	st     store.Store
	runner Runner
	log    *log.Logger
	tmpl   *template.Template
}

func NewServer(llmModel string, st store.Store, runner Runner, lg *log.Logger) (*Server, error) {
	t, err := template.New("").Funcs(template.FuncMap{
		"shortTime": func(tm time.Time) string {
			if tm.IsZero() {
				return "—"
			}
			return tm.Format("Jan 2 15:04")
		},
	}).ParseFS(tmplFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Server{model: llmModel, st: st, runner: runner, log: lg, tmpl: t}, nil
}

// Handler returns the configured router.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.dashboard)
	mux.HandleFunc("POST /run", s.run)
	mux.HandleFunc("POST /seeds/add", s.seedAdd)
	mux.HandleFunc("POST /seeds/remove", s.seedRemove)
	mux.HandleFunc("POST /keywords/add", s.keywordAdd)
	mux.HandleFunc("POST /niches/{id}/favorite", s.toggleFavorite)
	mux.HandleFunc("POST /niches/{id}/archive", s.toggleArchive)
	mux.HandleFunc("POST /niches/{id}/rescore", s.rescore)
	mux.HandleFunc("POST /niches/{id}/delete", s.deleteNiche)
	mux.HandleFunc("GET /api/niches", s.apiNiches)
	mux.HandleFunc("GET /api/niches.txt", s.apiNiches)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})
	return mux
}

type dashboardData struct {
	Model    string
	Status   niche.Status
	Seeds    []string
	Niches   []nicheView
	ShowAll  bool // include archived
	Archived int  // count hidden when !ShowAll
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	niches, err := s.st.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	seeds, err := s.st.Seeds()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	showAll := r.URL.Query().Get("show") == "all"
	data := dashboardData{Model: s.model, Status: s.runner.Status(), Seeds: seeds, ShowAll: showAll}
	for _, n := range niches {
		if n.Archived && !showAll {
			data.Archived++
			continue
		}
		data.Niches = append(data.Niches, toView(n))
	}
	// Favorites float to the top; List() already ranked by opportunity within.
	sort.SliceStable(data.Niches, func(i, j int) bool {
		return data.Niches[i].Favorite && !data.Niches[j].Favorite
	})
	s.render(w, "dashboard.html", data)
}

func (s *Server) run(w http.ResponseWriter, r *http.Request) {
	if !s.runner.Start(r.FormValue("category")) {
		s.log.Printf("run requested while one already in progress")
	}
	s.back(w, r)
}

func (s *Server) seedAdd(w http.ResponseWriter, r *http.Request) {
	if err := s.st.AddSeed(r.FormValue("category")); err != nil {
		s.log.Printf("add seed: %v", err)
	}
	s.back(w, r)
}

func (s *Server) seedRemove(w http.ResponseWriter, r *http.Request) {
	if err := s.st.RemoveSeed(r.FormValue("category")); err != nil {
		s.log.Printf("remove seed: %v", err)
	}
	s.back(w, r)
}

func (s *Server) keywordAdd(w http.ResponseWriter, r *http.Request) {
	s.runner.AddKeyword(r.FormValue("keyword"))
	s.back(w, r)
}

func (s *Server) rescore(w http.ResponseWriter, r *http.Request) {
	s.runner.Rescore(r.PathValue("id"))
	s.back(w, r)
}

func (s *Server) deleteNiche(w http.ResponseWriter, r *http.Request) {
	if err := s.st.Delete(r.PathValue("id")); err != nil {
		s.log.Printf("delete niche: %v", err)
	}
	s.back(w, r)
}

func (s *Server) toggleFavorite(w http.ResponseWriter, r *http.Request) {
	s.mutate(r.PathValue("id"), func(n *model.Niche) { n.Favorite = !n.Favorite })
	s.back(w, r)
}

func (s *Server) toggleArchive(w http.ResponseWriter, r *http.Request) {
	s.mutate(r.PathValue("id"), func(n *model.Niche) { n.Archived = !n.Archived })
	s.back(w, r)
}

// mutate loads a niche, applies fn, and saves it.
func (s *Server) mutate(id string, fn func(*model.Niche)) {
	n, err := s.st.Get(id)
	if err != nil {
		s.log.Printf("mutate %s: %v", id, err)
		return
	}
	fn(n)
	if err := s.st.Save(n); err != nil {
		s.log.Printf("save %s: %v", id, err)
	}
}

// back redirects to the referring page (preserves the ?show=all filter) or home.
func (s *Server) back(w http.ResponseWriter, r *http.Request) {
	dest := r.Referer()
	if dest == "" {
		dest = "/"
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		s.log.Printf("render %s: %v", name, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
