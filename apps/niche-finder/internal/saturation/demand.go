package saturation

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Demand is measured for free via Google autocomplete: a phrase that Google
// completes into many real-search suggestions has search demand; one that yields
// nothing is likely dead. It needs no API key. (A cleaner stand-in for the
// brittle Google-Trends/Etsy-autosuggest endpoints.)

var demandClient = &http.Client{Timeout: 8 * time.Second}

// autocompleteDemand returns a 0-100 demand score and the raw suggestion count,
// or (-1, 0, err) when the endpoint is unreachable so the composite can adapt.
//
// Very specific long-tail phrases (the ones we *want*) often have no direct
// autocomplete; to avoid under-rating them, a phrase with no completions falls
// back to probing its 3-word root so it gets (discounted) credit for the
// category's demand.
func autocompleteDemand(ctx context.Context, phrase string) (int, int, error) {
	score, n, err := suggestOnce(ctx, phrase)
	if err != nil {
		return -1, 0, err
	}
	if n == 0 {
		if words := strings.Fields(phrase); len(words) > 3 {
			root := strings.Join(words[:3], " ")
			if rs, rn, rerr := suggestOnce(ctx, root); rerr == nil && rn > 0 {
				return clamp(rs-15, 0, 100), rn, nil // discount: broader than the exact phrase
			}
		}
	}
	return score, n, nil
}

// suggestOnce does one autocomplete lookup and returns (score, count, err).
func suggestOnce(ctx context.Context, query string) (int, int, error) {
	u := "https://suggestqueries.google.com/complete/search?client=firefox&hl=en&q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return -1, 0, err
	}
	// A browser-ish UA avoids the occasional bot rejection.
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; niche-finder/0.1)")
	resp, err := demandClient.Do(req)
	if err != nil {
		return -1, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return -1, 0, err
	}
	n, err := parseSuggestCount(body)
	if err != nil {
		return -1, 0, err
	}
	return suggestionDemandScore(n), n, nil
}

// parseSuggestCount pulls the suggestion count from the autocomplete payload,
// which is ["<query>", ["sug1","sug2",...], ...]. Split out for testing.
func parseSuggestCount(body []byte) (int, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return 0, err
	}
	if len(raw) < 2 {
		return 0, nil
	}
	var suggestions []string
	if err := json.Unmarshal(raw[1], &suggestions); err != nil {
		return 0, err
	}
	return len(suggestions), nil
}

// suggestionDemandScore maps suggestion count (0..~10) to a 0-100 demand proxy.
// More completions ⇒ the phrase is a live, frequently-typed search stem.
func suggestionDemandScore(n int) int {
	if n <= 0 {
		return 5 // Google offers nothing — little/no search demand
	}
	return clamp(15+n*9, 0, 100) // 1→24, 5→60, 10→100
}
