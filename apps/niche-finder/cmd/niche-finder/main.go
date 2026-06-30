// Command niche-finder discovers product niches and scores how saturated each
// one is. It mines signals from pluggable sources (Reddit, Google Trends, Etsy),
// asks the home LLM cluster (Qwen, via HAProxy) to distill them into concrete
// niches, measures competition for each, and serves a ranked web dashboard.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/3thanHead/iot_ai/niche-finder/internal/config"
	"github.com/3thanHead/iot_ai/niche-finder/internal/etsy"
	"github.com/3thanHead/iot_ai/niche-finder/internal/llm"
	"github.com/3thanHead/iot_ai/niche-finder/internal/niche"
	"github.com/3thanHead/iot_ai/niche-finder/internal/saturation"
	"github.com/3thanHead/iot_ai/niche-finder/internal/source"
	"github.com/3thanHead/iot_ai/niche-finder/internal/store"
	"github.com/3thanHead/iot_ai/niche-finder/internal/web"
)

func main() {
	lg := log.New(os.Stdout, "", log.LstdFlags)
	cfg := config.Load()

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		lg.Fatalf("create data dir: %v", err)
	}

	st, err := store.Open(filepath.Join(cfg.DataDir, "niches.db"))
	if err != nil {
		lg.Fatalf("open store: %v", err)
	}
	defer st.Close()

	llmClient, err := llm.New(cfg.LLMBaseURL, cfg.LLMModel)
	if err != nil {
		lg.Fatalf("llm: %v", err)
	}

	// Etsy is enabled only with an API key; it powers both a mining source and
	// the preferred (measured) saturation backend.
	var etsyClient *etsy.Client
	if cfg.EtsyAPIKey != "" {
		etsyClient = etsy.New(cfg.EtsyAPIKey)
	}

	sources := source.Build(cfg.NicheSources, cfg.EtsyAPIKey, cfg.MineLimit)
	scorer := saturation.NewScorer(cfg.SaturationMarkets, etsyClient, llmClient)

	// Bootstrap the managed seed list from config the first time (the dashboard
	// owns it after that).
	if existing, _ := st.Seeds(); len(existing) == 0 {
		for _, s := range cfg.SeedCategories {
			if err := st.AddSeed(s); err != nil {
				lg.Printf("seed %q: %v", s, err)
			}
		}
	}

	// Cancel on SIGINT/SIGTERM so an in-flight run + the HTTP server stop cleanly.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	finder := niche.NewFinder(ctx, sources, llmClient, scorer, st, lg)

	srv, err := web.NewServer(cfg.LLMModel, st, finder, lg)
	if err != nil {
		lg.Fatalf("web: %v", err)
	}

	httpSrv := &http.Server{Addr: cfg.Addr, Handler: srv.Handler()}
	go func() {
		lg.Printf("dashboard on %s  (LLM=%s model=%s, sources=%d, etsy=%t)",
			cfg.Addr, cfg.LLMBaseURL, cfg.LLMModel, len(sources), etsyClient != nil)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			lg.Fatalf("http: %v", err)
		}
	}()

	<-ctx.Done()
	lg.Printf("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutCtx)
}
