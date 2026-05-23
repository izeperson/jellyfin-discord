package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func searchAniList(ctx context.Context, query string) (posterURL string, score string) {
	anilistSearchCache.RLock()
	if cached, ok := anilistSearchCache.m[query]; ok && time.Since(cached.Timestamp) < CacheTTL {
		anilistSearchCache.RUnlock()
		return cached.PosterURL, cached.Score
	}
	anilistSearchCache.RUnlock()

	start := time.Now()
	jsonData := map[string]interface{}{
		"query": `
			query ($search: String) {
				Media (search: $search, type: ANIME) {
					coverImage { large }
					averageScore
				}
			}`,
		"variables": map[string]string{"search": query},
	}

	body, _ := json.Marshal(jsonData)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://graphql.anilist.co", bytes.NewBuffer(body))
	if err != nil {
		return "", ""
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			logWarn("AniList", fmt.Sprintf("API request failed: %v", err))
		}
		return "", ""
	}
	defer resp.Body.Close()

	duration := time.Since(start).Truncate(time.Millisecond)
	if resp.StatusCode != 200 {
		logWarn("AniList", fmt.Sprintf("API request failed in %v: status %d", duration, resp.StatusCode))
		return "", ""
	}

	var res struct {
		Data struct {
			Media struct {
				CoverImage struct {
					Large string `json:"large"`
				} `json:"coverImage"`
				AverageScore int `json:"averageScore"`
			} `json:"Media"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		if ctx.Err() == nil {
			logWarn("AniList", fmt.Sprintf("JSON decode error: %v", err))
		}
		return "", ""
	}

	posterURL = res.Data.Media.CoverImage.Large
	if res.Data.Media.AverageScore > 0 {
		score = fmt.Sprintf("❤️ %d%%", res.Data.Media.AverageScore)
	}
	if posterURL != "" {
		if ctx.Err() != nil {
			return "", ""
		}
		logInfo("AniList", fmt.Sprintf("Found artwork in %v: %s (Score: %s)", duration, posterURL, score))
		anilistSearchCache.Lock()
		anilistSearchCache.m[query] = struct {
			PosterURL string
			Score     string
			Timestamp time.Time
		}{PosterURL: posterURL, Score: score, Timestamp: time.Now()}
		anilistSearchCache.Unlock()
		return posterURL, score
	}
	if ctx.Err() != nil {
		return "", ""
	}
	logInfo("AniList", fmt.Sprintf("No artwork found in %v for query: %s", duration, query))
	anilistSearchCache.Lock()
	anilistSearchCache.m[query] = struct {
		PosterURL string
		Score     string
		Timestamp time.Time
	}{PosterURL: "", Score: "", Timestamp: time.Now()}
	anilistSearchCache.Unlock()
	return
}
