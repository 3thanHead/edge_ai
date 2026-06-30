package source

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// Reddit mines hot posts from a set of subreddits via the public *.json
// endpoints (no API key). Hobby/maker subs surface people describing what they
// want and what they made — fertile ground for niche ideas.
type Reddit struct {
	subs  []string
	limit int
}

func NewReddit(subs []string, limit int) *Reddit { return &Reddit{subs: subs, limit: limit} }

func (r *Reddit) Name() string { return "reddit" }

// redditListing is the slice of the response we use. Both the hot-posts and the
// search endpoints return this same shape.
type redditListing struct {
	Data struct {
		Children []struct {
			Data struct {
				Title     string `json:"title"`
				Selftext  string `json:"selftext"`
				Permalink string `json:"permalink"`
			} `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

func (r *Reddit) Mine(ctx context.Context) ([]Signal, error) {
	per := r.limit / max(len(r.subs), 1)
	if per < 1 {
		per = 1
	}
	var signals []Signal
	var firstErr error
	for _, sub := range r.subs {
		u := fmt.Sprintf("https://www.reddit.com/r/%s/hot.json?limit=%d", sub, per)
		body, err := fetch(ctx, u, nil)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue // one dead sub shouldn't sink the run
		}
		signals = append(signals, parseReddit(body, sub)...)
	}
	if len(signals) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return signals, nil
}

// SearchReddit runs a site-wide Reddit search for a query and returns the top
// matching posts as Signals. This is the "deepen" primitive: the research engine
// follows a lead term the LLM proposed, rather than only reading fixed subs.
func SearchReddit(ctx context.Context, query string, limit int) ([]Signal, error) {
	if limit < 1 {
		limit = 1
	}
	u := fmt.Sprintf("https://www.reddit.com/search.json?q=%s&sort=top&t=year&limit=%d",
		url.QueryEscape(query), limit)
	body, err := fetch(ctx, u, nil)
	if err != nil {
		return nil, err
	}
	return parseRedditPosts(body, "reddit-search:"+query), nil
}

// parseReddit turns a hot-listing payload into Signals labelled by subreddit.
func parseReddit(body []byte, sub string) []Signal {
	return parseRedditPosts(body, "reddit:"+sub)
}

// parseRedditPosts is the shared parser; sourceLabel tags where each signal came
// from (a subreddit for hot, or the query for search).
func parseRedditPosts(body []byte, sourceLabel string) []Signal {
	var listing redditListing
	if err := json.Unmarshal(body, &listing); err != nil {
		return nil
	}
	var out []Signal
	for _, c := range listing.Data.Children {
		d := c.Data
		if d.Title == "" {
			continue
		}
		u := ""
		if d.Permalink != "" {
			u = "https://www.reddit.com" + d.Permalink
		}
		out = append(out, Signal{
			Source: sourceLabel,
			Title:  d.Title,
			Text:   d.Selftext,
			URL:    u,
		})
	}
	return out
}
