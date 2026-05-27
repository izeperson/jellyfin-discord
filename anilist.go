package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

var (
	aniListCircuitOpenUntil time.Time
	aniListCircuitLock      sync.RWMutex
)

func searchAniList(ctx context.Context, query string) (posterURL string, score string, siteURL string) {
	aniListCircuitLock.RLock()
	if time.Now().Before(aniListCircuitOpenUntil) {
		aniListCircuitLock.RUnlock()
		logWarn("AniList", "Circuit breaker is open. Skipping request due to recent rate limiting.")
		return "", "", ""
	}
	aniListCircuitLock.RUnlock()

	anilistSearchCache.RLock()
	if cached, ok := anilistSearchCache.m[query]; ok && time.Since(cached.Timestamp) < CacheTTL {
		anilistSearchCache.RUnlock()
		return cached.PosterURL, cached.Score, cached.SiteURL
	}
	anilistSearchCache.RUnlock()

	start := time.Now()
	jsonData := map[string]interface{}{
		"query": `
			query ($search: String) {
				Media (search: $search, type: ANIME) {
					coverImage { large }
					siteUrl
					averageScore
				}
			}`,
		"variables": map[string]string{"search": query},
	}

	body, _ := json.Marshal(jsonData)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://graphql.anilist.co", bytes.NewBuffer(body))
	if err != nil {
		return "", "", ""
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			logWarn("AniList", fmt.Sprintf("API request failed: %v", err))
		}
		return "", "", ""
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		aniListCircuitLock.Lock()
		aniListCircuitOpenUntil = time.Now().Add(5 * time.Minute)
		aniListCircuitLock.Unlock()
		logError("AniList", "Received 429 Rate Limit. Opening circuit breaker for 5 minutes.")
		return "", "", ""
	}

	duration := time.Since(start).Truncate(time.Millisecond)
	if resp.StatusCode != 200 {
		logWarn("AniList", fmt.Sprintf("API request failed in %v: status %d", duration, resp.StatusCode))
		return "", "", ""
	}

	var res struct {
		Data struct {
			Media struct {
				CoverImage struct {
					Large string `json:"large"`
				} `json:"coverImage"`
				SiteUrl      string `json:"siteUrl"`
				AverageScore int    `json:"averageScore"`
			} `json:"Media"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		if ctx.Err() == nil {
			logWarn("AniList", fmt.Sprintf("JSON decode error: %v", err))
		}
		return "", "", ""
	}

	posterURL = res.Data.Media.CoverImage.Large
	siteURL = res.Data.Media.SiteUrl
	if res.Data.Media.AverageScore > 0 {
		score = fmt.Sprintf("❤️ %d%%", res.Data.Media.AverageScore)
	}
	if posterURL != "" {
		if ctx.Err() != nil {
			return "", "", ""
		}
		logInfo("AniList", fmt.Sprintf("Found artwork in %v: %s (Score: %s)", duration, posterURL, score))
		anilistSearchCache.Lock()
		anilistSearchCache.m[query] = struct {
			PosterURL string
			Score     string
			SiteURL   string
			Timestamp time.Time
		}{PosterURL: posterURL, Score: score, SiteURL: siteURL, Timestamp: time.Now()}
		anilistSearchCache.Unlock()
		return posterURL, score, siteURL
	}
	if ctx.Err() != nil {
		return "", "", ""
	}
	logInfo("AniList", fmt.Sprintf("No artwork found in %v for query: %s", duration, query))
	anilistSearchCache.Lock()
	anilistSearchCache.m[query] = struct {
		PosterURL string
		Score     string
		SiteURL   string
		Timestamp time.Time
	}{PosterURL: "", Score: "", SiteURL: "", Timestamp: time.Now()}
	anilistSearchCache.Unlock()
	return
}
