package web

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/3thanHead/iot_ai/niche-finder/internal/model"
	"github.com/3thanHead/iot_ai/niche-finder/internal/niche"
	"github.com/3thanHead/iot_ai/niche-finder/internal/saturation"
	"github.com/3thanHead/iot_ai/niche-finder/internal/store"
)

type stubRunner struct{}

func (stubRunner) Start(string) bool    { return false }
func (stubRunner) Status() niche.Status { return niche.Status{} }
func (stubRunner) AddKeyword(string)    {}
func (stubRunner) Rescore(string)       {}

func newTestServer(t *testing.T) (*Server, store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "n.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	srv, err := NewServer("qwen-test", st, stubRunner{}, log.New(nil, "", 0))
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	return srv, st
}

func sampleNiche() *model.Niche {
	return &model.Niche{
		ID: "n1", Name: "Keto Printable Cookbook", Theme: "Cookbooks",
		Audience: "keto dieters", ProductAngle: "printable PDF cookbook", Opportunity: 58,
		Keywords: []model.Keyword{
			{Phrase: "printable keto meal prep cookbook", Type: "long-tail", Opportunity: 58,
				Saturation: saturation.Result{Value: 42, Method: "measured", Competitors: 1203, Source: "etsy"}},
			{Phrase: "keto cookbook", Type: "short-tail", Opportunity: 12,
				Saturation: saturation.Result{Value: 88, Method: "estimated", Source: "llm"}},
		},
	}
}

func TestDashboardRendersKeywords(t *testing.T) {
	srv, st := newTestServer(t)
	if err := st.Save(sampleNiche()); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard status %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"Keto Printable Cookbook", "printable keto meal prep cookbook", "long-tail", "keto cookbook"} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard missing %q", want)
		}
	}
}

func TestAPINichesJSON(t *testing.T) {
	srv, st := newTestServer(t)
	if err := st.Save(sampleNiche()); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/niches", nil))
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content-type %q", ct)
	}
	var got []apiNiche
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || len(got[0].Keywords) != 2 {
		t.Fatalf("got %d niches; keywords %v", len(got), got)
	}
	if got[0].Keywords[0].Phrase != "printable keto meal prep cookbook" {
		t.Errorf("first keyword = %q", got[0].Keywords[0].Phrase)
	}
}

func TestAPINichesTextAndArchiveFilter(t *testing.T) {
	srv, st := newTestServer(t)
	n := sampleNiche()
	n.Archived = true
	if err := st.Save(n); err != nil {
		t.Fatal(err)
	}
	// Archived niche hidden by default...
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/niches.txt", nil))
	if !strings.Contains(rr.Body.String(), "0 niches") {
		t.Errorf("archived niche should be hidden by default: %q", rr.Body.String())
	}
	// ...shown with ?show=all, and the text lists the keyword.
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/niches.txt?show=all", nil))
	if !strings.Contains(rr.Body.String(), "keto cookbook") {
		t.Errorf("text output missing keyword: %q", rr.Body.String())
	}
}
