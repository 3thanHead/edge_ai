package source

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// userAgent identifies the miner politely. Reddit's public JSON in particular
// rejects requests with a default/empty UA, so set a descriptive one.
const userAgent = "niche-finder/0.1 (https://github.com/3thanHead/iot_ai)"

// httpClient is shared by all adapters; a short timeout keeps a slow or blocking
// source from stalling a whole mining run.
var httpClient = &http.Client{Timeout: 12 * time.Second}

// fetch does a GET with the right headers and returns the body bytes. Non-2xx is
// an error so adapters can fail soft (a dead source contributes nothing).
func fetch(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // 4 MiB cap
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return body, nil
}
