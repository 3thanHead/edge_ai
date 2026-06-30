package source

import "testing"

func TestParseReddit(t *testing.T) {
	body := []byte(`{"data":{"children":[
		{"data":{"title":"I made a custom dog bandana","selftext":"with embroidery","permalink":"/r/somethingimade/comments/abc/"}},
		{"data":{"title":"","selftext":"skip me"}},
		{"data":{"title":"Anyone into resin keychains?","selftext":""}}
	]}}`)
	got := parseReddit(body, "somethingimade")
	if len(got) != 2 {
		t.Fatalf("got %d signals, want 2 (empty-title dropped)", len(got))
	}
	if got[0].Title != "I made a custom dog bandana" || got[0].Source != "reddit:somethingimade" {
		t.Errorf("unexpected first signal: %+v", got[0])
	}
	if got[0].URL != "https://www.reddit.com/r/somethingimade/comments/abc/" {
		t.Errorf("permalink not expanded: %q", got[0].URL)
	}
}

func TestParseRedditPostsLabel(t *testing.T) {
	// The search path reuses parseRedditPosts with a query-based source label.
	body := []byte(`{"data":{"children":[{"data":{"title":"printable budget planner"}}]}}`)
	got := parseRedditPosts(body, "reddit-search:budget planner")
	if len(got) != 1 || got[0].Source != "reddit-search:budget planner" {
		t.Fatalf("unexpected label/result: %+v", got)
	}
}

func TestParseRedditBadJSON(t *testing.T) {
	if got := parseReddit([]byte("not json"), "x"); got != nil {
		t.Errorf("expected nil on bad JSON, got %v", got)
	}
}

func TestParseTrends(t *testing.T) {
	body := []byte(`<?xml version="1.0"?><rss><channel>
		<item><title>cold plunge tub</title><ht:approx_traffic>50,000+</ht:approx_traffic>
			<ht:news_item><ht:news_item_title>Cold plunge craze</ht:news_item_title>
			<ht:news_item_snippet>everyone is doing it</ht:news_item_snippet></ht:news_item>
		</item>
		<item><title>sourdough starter kit</title></item>
	</channel></rss>`)
	got := parseTrends(body, 10)
	if len(got) != 2 {
		t.Fatalf("got %d signals, want 2", len(got))
	}
	if got[0].Title != "cold plunge tub" || got[0].Source != "gtrends" {
		t.Errorf("unexpected first trend: %+v", got[0])
	}
	if got[0].Text == "" {
		t.Errorf("expected news context folded into Text, got empty")
	}
}

func TestParseTrendsLimit(t *testing.T) {
	body := []byte(`<rss><channel>
		<item><title>a</title></item><item><title>b</title></item><item><title>c</title></item>
	</channel></rss>`)
	if got := parseTrends(body, 2); len(got) != 2 {
		t.Errorf("limit not applied: got %d, want 2", len(got))
	}
}

func TestBuildSources(t *testing.T) {
	// reddit + gtrends from spec, etsy added by key.
	srcs := Build("reddit:a,b;gtrends:", "KEY123", 20)
	if len(srcs) != 3 {
		t.Fatalf("got %d sources, want 3", len(srcs))
	}
	// No key => Etsy skipped.
	srcs = Build("reddit:a", "", 20)
	if len(srcs) != 1 {
		t.Fatalf("got %d sources without key, want 1", len(srcs))
	}
	// Empty reddit args => adapter skipped.
	srcs = Build("reddit:;gtrends:", "", 20)
	if len(srcs) != 1 || srcs[0].Name() != "gtrends" {
		t.Fatalf("empty reddit should be skipped, got %+v", srcs)
	}
}
