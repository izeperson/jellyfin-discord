package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var itunesSearchCache = struct {
	sync.RWMutex
	m map[string]struct {
		Value     string
		Timestamp time.Time
	}
}{m: make(map[string]struct {
	Value     string
	Timestamp time.Time
})}

func searchiTunes(ctx context.Context, query string, expectedArtist string) string {
	cacheKey := query + "|" + expectedArtist
	itunesSearchCache.RLock()
	if cached, ok := itunesSearchCache.m[cacheKey]; ok && time.Since(cached.Timestamp) < CacheTTL {
		itunesSearchCache.RUnlock()
		return cached.Value
	}
	itunesSearchCache.RUnlock()
	start := time.Now()
	searchURL := fmt.Sprintf("https://itunes.apple.com/search?term=%s&entity=song&limit=1", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return ""
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			logWarn("iTunes", fmt.Sprintf("API request failed: %v", err))
		}
		return ""
	}
	defer resp.Body.Close()

	duration := time.Since(start).Truncate(time.Millisecond)
	if resp.StatusCode != 200 {
		logWarn("iTunes", fmt.Sprintf("API request failed in %v: status %d", duration, resp.StatusCode))
		return ""
	}

	var res struct {
		Results []struct {
			ArtistName    string `json:"artistName"`
			ArtworkUrl100 string `json:"artworkUrl100"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		if ctx.Err() == nil {
			logWarn("iTunes", fmt.Sprintf("JSON decode error: %v", err))
		}
		return ""
	}

	if len(res.Results) > 0 {
		if expectedArtist != "" && !strings.EqualFold(res.Results[0].ArtistName, expectedArtist) {
			logWarn("iTunes", fmt.Sprintf("Artist mismatch in %v for '%s': expected %s, found %s.", duration, query, expectedArtist, res.Results[0].ArtistName))
			itunesSearchCache.Lock()
			itunesSearchCache.m[cacheKey] = struct {
				Value     string
				Timestamp time.Time
			}{Value: "", Timestamp: time.Now()}
			itunesSearchCache.Unlock()
			return ""
		}

		artwork := res.Results[0].ArtworkUrl100
		posterURL := strings.Replace(artwork, "100x100bb", "512x512bb", 1)
		if ctx.Err() != nil {
			return ""
		}
		logInfo("iTunes", fmt.Sprintf("Found artwork in %v: %s", duration, posterURL))

		itunesSearchCache.Lock()
		itunesSearchCache.m[cacheKey] = struct {
			Value     string
			Timestamp time.Time
		}{Value: posterURL, Timestamp: time.Now()}
		itunesSearchCache.Unlock()
		return posterURL
	}
	if ctx.Err() != nil {
		return ""
	}
	logInfo("iTunes", fmt.Sprintf("No artwork found in %v for query: %s", duration, query))
	itunesSearchCache.Lock()
	itunesSearchCache.m[cacheKey] = struct {
		Value     string
		Timestamp time.Time
	}{Value: "", Timestamp: time.Now()}
	itunesSearchCache.Unlock()
	return ""
}
