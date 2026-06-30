// Package etsy is a tiny client for the one Etsy Open API v3 endpoint we use:
// "find all active listings". It needs only an API keystring (the x-api-key
// header) — no OAuth — because active-listing search is app-key auth. The same
// call serves two callers: the source miner (listing titles/tags as signals)
// and the saturation scorer (the `count` field = competing active listings).
package etsy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const base = "https://openapi.etsy.com/v3/application/listings/active"

// Client holds the API key. The zero value is unusable; use New.
type Client struct {
	key  string
	http *http.Client
}

func New(apiKey string) *Client {
	return &Client{key: apiKey, http: &http.Client{Timeout: 12 * time.Second}}
}

// Listing is the slice of an Etsy listing we care about.
type Listing struct {
	Title string   `json:"title"`
	Tags  []string `json:"tags"`
	URL   string   `json:"url"`
}

type activeResponse struct {
	Count   int       `json:"count"`
	Results []Listing `json:"results"`
}

// ActiveListings returns the total match count and up to `limit` listings for a
// keyword query. Count is the saturation signal; the listings are mining input.
func (c *Client) ActiveListings(ctx context.Context, keywords string, limit int) (int, []Listing, error) {
	if c.key == "" {
		return 0, nil, fmt.Errorf("etsy: no API key")
	}
	if limit < 1 {
		limit = 1
	}
	q := url.Values{}
	q.Set("keywords", keywords)
	q.Set("limit", strconv.Itoa(limit))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"?"+q.Encode(), nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("x-api-key", c.key)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, nil, fmt.Errorf("etsy: %s: %s", resp.Status, string(body))
	}
	var out activeResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return 0, nil, fmt.Errorf("etsy: parse: %w", err)
	}
	return out.Count, out.Results, nil
}
