package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

var musicbrainzSearchCache = struct {
	sync.RWMutex
	m map[string]struct {
		Value     string
		Timestamp time.Time
	}
}{m: make(map[string]struct {
	Value     string
	Timestamp time.Time
})}

var (
	mbCircuitOpenUntil time.Time
	mbCircuitLock      sync.RWMutex
)

func searchMusicBrainz(ctx context.Context, query string, expectedArtist string) string {
	mbCircuitLock.RLock()
	if time.Now().Before(mbCircuitOpenUntil) {
		mbCircuitLock.RUnlock()
		logWarn("MusicBrainz", "Circuit breaker is open. Skipping request due to recent rate limiting (503).")
		return ""
	}
	mbCircuitLock.RUnlock()

	cacheKey := query + "|" + expectedArtist
	musicbrainzSearchCache.RLock()
	if cached, ok := musicbrainzSearchCache.m[cacheKey]; ok && time.Since(cached.Timestamp) < CacheTTL {
		musicbrainzSearchCache.RUnlock()
		return cached.Value
	}
	musicbrainzSearchCache.RUnlock()

	start := time.Now()

	var mbQuery string
	if expectedArtist != "" {
		mbQuery = fmt.Sprintf("artist:\"%s\" AND (release:\"%s\" OR \"%s\")", expectedArtist, query, query)
	} else {
		mbQuery = query
	}

	searchURL := fmt.Sprintf("https://musicbrainz.org/ws/2/release/?query=%s&fmt=json&limit=1", url.QueryEscape(mbQuery))

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return ""
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			logWarn("MusicBrainz", fmt.Sprintf("API request failed: %v", err))
		}
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode == 503 {
		mbCircuitLock.Lock()
		mbCircuitOpenUntil = time.Now().Add(5 * time.Minute)
		mbCircuitLock.Unlock()
		logError("MusicBrainz", "Received 503 Rate Limit. Opening circuit breaker for 5 minutes.")
		return ""
	}

	duration := time.Since(start).Truncate(time.Millisecond)
	if resp.StatusCode != 200 {
		logWarn("MusicBrainz", fmt.Sprintf("API request failed in %v: status %d", duration, resp.StatusCode))
		return ""
	}

	var res struct {
		Releases []struct {
			ID string `json:"id"`
		} `json:"releases"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		if ctx.Err() == nil {
			logWarn("MusicBrainz", fmt.Sprintf("JSON decode error: %v", err))
		}
		return ""
	}

	if len(res.Releases) > 0 {
		mbid := res.Releases[0].ID
		posterURL := fmt.Sprintf("https://coverartarchive.org/release/%s/front", mbid)
		if ctx.Err() != nil {
			return ""
		}
		logInfo("MusicBrainz", fmt.Sprintf("Found artwork in %v: %s", duration, mbid))

		musicbrainzSearchCache.Lock()
		musicbrainzSearchCache.m[cacheKey] = struct {
			Value     string
			Timestamp time.Time
		}{Value: posterURL, Timestamp: time.Now()}
		musicbrainzSearchCache.Unlock()
		return posterURL
	}

	if ctx.Err() != nil {
		return ""
	}
	logInfo("MusicBrainz", fmt.Sprintf("No artwork found in %v for query: %s", duration, query))
	musicbrainzSearchCache.Lock()
	musicbrainzSearchCache.m[cacheKey] = struct {
		Value     string
		Timestamp time.Time
	}{Value: "", Timestamp: time.Now()}
	musicbrainzSearchCache.Unlock()
	return ""
}

var _ = time.Second
