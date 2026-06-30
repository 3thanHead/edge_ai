// Package source mines raw text signals from external sites — the input the LLM
// later distills into niche candidates. Each backend is a small Source adapter
// behind one interface, so new sites slot in without touching the finder.
package source

import (
	"context"
	"strings"
)

// Signal is one raw item pulled from a source: a title plus optional body text
// and a link back to the origin. The LLM reads many of these to spot niches.
type Signal struct {
	Source string // adapter name, e.g. "reddit"
	Title  string
	Text   string
	URL    string
}

// Source is a single mineable backend.
type Source interface {
	Name() string
	Mine(ctx context.Context) ([]Signal, error)
}

// Build parses the NICHE_SOURCES spec and returns the enabled adapters.
//
// Spec grammar: semicolon-separated entries of "name:arg,arg,...". Today:
//
//	reddit:sub1,sub2,...   one subreddit per arg
//	gtrends:               (no args) trending search queries
//
// The Etsy adapter is not spec-driven: it's added automatically when etsyKey is
// non-empty (it needs the official API key, not just a flag). limit caps items
// pulled per source.
func Build(spec, etsyKey string, limit int) []Source {
	var sources []Source
	for _, entry := range strings.Split(spec, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		name, rawArgs, _ := strings.Cut(entry, ":")
		name = strings.TrimSpace(strings.ToLower(name))
		args := splitArgs(rawArgs)
		switch name {
		case "reddit":
			if len(args) > 0 {
				sources = append(sources, NewReddit(args, limit))
			}
		case "gtrends", "trends":
			sources = append(sources, NewGoogleTrends(limit))
		}
	}
	if etsyKey != "" {
		sources = append(sources, NewEtsy(etsyKey, limit))
	}
	return sources
}

func splitArgs(raw string) []string {
	var out []string
	for _, a := range strings.Split(raw, ",") {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	return out
}
