package saturation

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ebayClient uses a browser-like User-Agent; eBay serves a stripped page (or
// blocks) requests that look like bots.
var ebayClient = &http.Client{Timeout: 12 * time.Second}

const ebayUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/124.0 Safari/537.36"

// countHeading isolates the result-count block; the count is the first run of
// digits/commas after it (e.g. `...count-heading"><span class="BOLD">12,345</span> results`).
var countHeading = regexp.MustCompile(`srp-controls__count-heading[\s\S]{0,200}?([\d,]+)`)

// ebayCount scrapes the number of active listings matching a keyword from an
// eBay search results page. Returns an error if the page can't be parsed (the
// scorer then falls through to the next market / LLM estimate).
func ebayCount(ctx context.Context, keyword string) (int, error) {
	u := "https://www.ebay.com/sch/i.html?_nkw=" + url.QueryEscape(keyword)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", ebayUA)
	req.Header.Set("Accept", "text/html")
	resp, err := ebayClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 6<<20))
	if err != nil {
		return 0, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("ebay: %s", resp.Status)
	}
	return parseEbayCount(string(body))
}

// parseEbayCount extracts the result count from eBay search HTML (split out for
// testing without a network call).
func parseEbayCount(html string) (int, error) {
	m := countHeading.FindStringSubmatch(html)
	if m == nil {
		return 0, fmt.Errorf("ebay: result count not found")
	}
	n, err := strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
	if err != nil {
		return 0, fmt.Errorf("ebay: bad count %q: %w", m[1], err)
	}
	return n, nil
}
