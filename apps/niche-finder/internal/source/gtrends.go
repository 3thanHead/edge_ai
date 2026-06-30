package source

import (
	"context"
	"encoding/xml"
	"strings"
)

// GoogleTrends mines today's trending search queries from the public Daily
// Trends RSS feed (no API key). Breakout queries are an early demand signal.
type GoogleTrends struct {
	limit int
	geo   string
}

func NewGoogleTrends(limit int) *GoogleTrends { return &GoogleTrends{limit: limit, geo: "US"} }

func (g *GoogleTrends) Name() string { return "gtrends" }

// trendsRSS captures the bits of the daily-trends feed we use. The ht:* fields
// are namespaced in the XML; encoding/xml matches on the local name.
type trendsRSS struct {
	Channel struct {
		Items []struct {
			Title    string `xml:"title"`
			Traffic  string `xml:"approx_traffic"`
			NewsItem []struct {
				Title   string `xml:"news_item_title"`
				Snippet string `xml:"news_item_snippet"`
			} `xml:"news_item"`
		} `xml:"item"`
	} `xml:"channel"`
}

func (g *GoogleTrends) Mine(ctx context.Context) ([]Signal, error) {
	url := "https://trends.google.com/trends/trendingsearches/daily/rss?geo=" + g.geo
	body, err := fetch(ctx, url, map[string]string{"Accept": "application/rss+xml, application/xml"})
	if err != nil {
		return nil, err
	}
	return parseTrends(body, g.limit), nil
}

// parseTrends turns the RSS payload into Signals (split out for testing).
func parseTrends(body []byte, limit int) []Signal {
	var feed trendsRSS
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil
	}
	var out []Signal
	for _, it := range feed.Channel.Items {
		if it.Title == "" {
			continue
		}
		// Fold the related news headlines/snippets into the body so the LLM has
		// context for what the trending term is actually about.
		var ctx []string
		for _, n := range it.NewsItem {
			if n.Title != "" {
				ctx = append(ctx, n.Title)
			}
			if n.Snippet != "" {
				ctx = append(ctx, n.Snippet)
			}
		}
		out = append(out, Signal{
			Source: "gtrends",
			Title:  strings.TrimSpace(it.Title),
			Text:   strings.Join(ctx, " — "),
			URL:    "https://trends.google.com/trends/trendingsearches/daily?geo=US",
		})
		if len(out) >= limit {
			break
		}
	}
	return out
}
